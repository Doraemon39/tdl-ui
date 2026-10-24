<template>
  <Dialog :visible="visible" modal :header="`${title} · 已下载清单`" :style="{ width: '780px', maxWidth: '96vw' }" :closable="!exporting" @update:visible="close">
    <p class="hint">扫描完整对话，并比对指定本地目录。清单仅包含确认匹配的记录，保存到本地，不含登录凭证。</p>
    <div class="form-field"><label>本地媒体目录</label><DirSelect v-model="localDir" kind="download" :disabled="busy || exporting" /></div>
    <div class="actions">
      <Button label="扫描并比对" icon="pi pi-search" :loading="busy" :disabled="!ready || !localDir || exporting" @click="scanAndCompare" />
      <Button v-if="busy" label="取消扫描" text severity="secondary" @click="cancelWork" />
      <Button label="导入已有清单" outlined severity="secondary" :disabled="busy || exporting" @click="importExisting" />
    </div>
    <p v-if="busy" role="status">已扫描 {{ scannedCount }} 项…</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <template v-if="comparison">
      <p>共 {{ comparison.total }} 项：匹配 {{ comparison.matched }}，未匹配 {{ comparison.unmatched }}，重名待核对 {{ comparison.ambiguous }}。</p>
      <Message severity="info" :closable="false">无下载记录的单一原名匹配按文件名和字节大小判断。请确认此目录确实来自当前对话，再导出清单。</Message>
      <DuplicateReport :comparison="comparison" :confirmed-ids="confirmedIds" reviewable @review="review" />
      <div v-for="choice in choices" :key="choice.messageId" class="choice">消息 {{ choice.messageId }} → {{ choice.path }} <Button label="撤销确认" text size="small" @click="choices = choices.filter(c => c.messageId !== choice.messageId)" /></div>
      <label class="ack"><Checkbox v-model="acknowledged" binary />我已核对本地目录与当前对话的来源一致</label>
    </template>
    <p v-if="active" class="hint">当前清单：{{ active.info.path }}（{{ active.info.done }} 项）</p>
    <template #footer>
      <Button label="关闭" severity="secondary" text :disabled="exporting" @click="close(false)" />
      <Button label="导出确认匹配项" icon="pi pi-file-export" :loading="exporting" :disabled="busy || !comparison || !acknowledged || comparison.matched + choices.length === 0" @click="exportConfirmed" />
    </template>
  </Dialog>
  <Dialog v-model:visible="reviewVisible" modal header="核对日期与本地文件" :style="{ width: '540px', maxWidth: '95vw' }">
    <template v-if="reviewEntry">
      <p>{{ reviewEntry.name }}</p><p>消息 {{ reviewEntry.messageId }} · {{ fmtMessageTime(reviewEntry.date) }} · {{ fmtSize(reviewEntry.size) }}</p>
      <Select v-model="reviewPath" :options="reviewFiles.filter(f => f.size === reviewEntry?.size)" option-label="path" option-value="path" placeholder="选择对应的本地文件" fluid />
      <label class="ack"><Checkbox v-model="reviewAcknowledged" binary />我已核对文件内容及上述消息日期，确认一一对应</label>
    </template>
    <template #footer><Button label="取消" text severity="secondary" @click="reviewVisible = false" /><Button label="确认对应" :disabled="!reviewAcknowledged || !reviewPath" @click="confirmReview" /></template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Select from 'primevue/select'
import DirSelect from './DirSelect.vue'
import DuplicateReport from './DuplicateReport.vue'
import { Download } from '../api'
import { manifest } from '../../wailsjs/go/models'
import type { ManifestInfo, MediaChoice, MediaComparison, MediaItem } from '../types'
import { manifestScopeMatches, type ManifestSelection } from '../utils/manifests'
import { fmtMessageTime, fmtSize } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { useSettingsStore } from '../stores/settings'

const props = defineProps<{ visible: boolean; dialogId: number; dialogType: string; title: string; ready: boolean; scannedCount: number; active: ManifestSelection | null; scan: () => Promise<{ status: string; items: MediaItem[] }>; cancel: () => void }>()
const emit = defineEmits<{ 'update:visible': [boolean]; 'update:active': [ManifestSelection | null] }>()
const auth = useAuthStore(), settings = useSettingsStore()
const localDir = ref(settings.settings.downloadDir || ''), busy = ref(false), exporting = ref(false), error = ref(''), acknowledged = ref(false)
const comparison = ref<MediaComparison | null>(null), choices = ref<MediaChoice[]>([])
const reviewVisible = ref(false), reviewEntry = ref<ManifestInfo['entries'][number] | null>(null), reviewFiles = ref<MediaComparison['duplicates'][number]['files']>([]), reviewPath = ref(''), reviewAcknowledged = ref(false)
const confirmedIds = computed(() => [...(comparison.value?.matches.map(m => m.entry.messageId) ?? []), ...choices.value.map(c => c.messageId)])
let epoch = 0
function cancelWork() { epoch++; props.cancel(); busy.value = false }
function close(v: boolean) { if (exporting.value) return; if (!v) cancelWork(); emit('update:visible', v) }
watch([() => props.visible, () => props.dialogId, () => props.dialogType, () => auth.userId, localDir], () => {
  cancelWork(); comparison.value = null; choices.value = []; error.value = ''; acknowledged.value = false; reviewVisible.value = false
  if (props.visible && !localDir.value) localDir.value = settings.settings.downloadDir || ''
})
async function scanAndCompare() {
  const mine = ++epoch, owner = auth.userId, dialog = props.dialogId, typ = props.dialogType, dir = localDir.value.trim()
  const valid = () => mine === epoch && props.visible && props.ready && owner === auth.userId && props.dialogId === dialog && props.dialogType === typ
  busy.value = true; comparison.value = null; choices.value = []; error.value = ''; acknowledged.value = false
  try {
    const result = await props.scan()
    if (!valid()) return
    if (result.status !== 'complete') throw new Error(result.status === 'limit' ? '扫描达到上限，请缩小范围后重试' : '扫描未完整完成，没有生成比对结果')
    const cmp = await Download.reviewLocalMedia(dir, dialog, typ, props.title, result.items)
    if (valid()) comparison.value = cmp
  } catch (e) { if (valid()) error.value = String(e) }
  finally { if (mine === epoch) busy.value = false }
}
function review(entry: ManifestInfo['entries'][number], files: MediaComparison['duplicates'][number]['files']) {
  reviewEntry.value = entry; reviewFiles.value = files; reviewPath.value = ''; reviewAcknowledged.value = false; reviewVisible.value = true
}
function confirmReview() {
  const entry = reviewEntry.value
  if (!entry || !reviewAcknowledged.value || !reviewPath.value) return
  choices.value = [...choices.value.filter(c => c.messageId !== entry.messageId), manifest.LocalChoice.createFrom({ messageId: entry.messageId, mediaId: entry.mediaId, date: entry.date, path: reviewPath.value, confirmed: true })]
  reviewVisible.value = false
}
async function importExisting() {
  const mine = epoch
  try {
    const info = await Download.chooseManifest()
    if (!info || mine !== epoch || !props.visible) return
    if (!manifestScopeMatches(info, auth.userId, props.dialogId, props.dialogType)) throw new Error('清单不属于当前账号或当前对话')
    emit('update:active', { info })
  } catch (e) { error.value = String(e) }
}
async function exportConfirmed() {
  if (!comparison.value || !acknowledged.value) return
  const mine = epoch, token = comparison.value.token
  exporting.value = true; error.value = ''
  try {
    const dir = await Download.selectDirectory('manifest', '')
    if (!dir || mine !== epoch) return
    const info = await Download.exportReviewedManifest(token, dir, choices.value.map(c => manifest.LocalChoice.createFrom(c)))
    if (mine === epoch && props.visible) emit('update:active', { info })
  } catch (e) { if (mine === epoch) error.value = String(e) }
  finally { exporting.value = false }
}
</script>

<style scoped>
.actions,.ack { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 16px 0; }
.choice { overflow-wrap: anywhere; font-size: 13px; }
</style>
