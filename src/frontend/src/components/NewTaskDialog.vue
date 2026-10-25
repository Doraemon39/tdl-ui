<template>
  <Dialog
    :visible="visible"
    modal
    header="添加下载"
    :style="{ width: '560px' }"
    @update:visible="(v: boolean) => emit('update:visible', v)"
  >
    <div v-if="selection" class="selection-summary">
      <i class="pi pi-images" />
      <span>候选范围：「<b>{{ selection.title }}</b>」共 <b>{{ selection.messageIds.length }}</b> 项，下载前统一比对</span>
    </div>

    <div v-else class="form-field">
      <label for="nt-urls">消息链接（每行一条）</label>
      <Textarea
        id="nt-urls"
        v-model="urls"
        rows="6"
        auto-resize
        placeholder="https://t.me/channel_name/123&#10;https://t.me/c/1234567890/456"
      />
      <span class="hint">支持公开频道与私有频道（t.me/c/...）消息链接</span>
    </div>

    <div class="form-field">
      <label for="nt-dir">保存目录</label>
      <DirSelect id="nt-dir" ref="dirSelect" v-model="dir" kind="download" placeholder="选择或输入下载保存目录" />
    </div>

    <!-- 文件重命名（选集模式且有媒体项时显示） -->
    <div v-if="selection?.items?.length" class="form-field">
      <div class="rename-header" @click="renameExpanded = !renameExpanded">
        <i class="pi" :class="renameExpanded ? 'pi-chevron-down' : 'pi-chevron-right'" />
        <span>文件重命名（可选）</span>
        <span v-if="renameCount" class="rename-count">{{ renameCount }} 项已自定义</span>
      </div>
      <div v-show="renameExpanded" class="rename-list">
        <div v-if="selection.items.length > 100" class="rename-limit-tip">
          <i class="pi pi-info-circle mr-1" />共 {{ selection.items.length }} 项，仅展示前 100 项；批量命名建议使用命名模板或脚本
        </div>
        <div v-for="(item, idx) in visibleRenameItems" :key="item.messageId" class="rename-row">
          <span class="rename-idx">{{ idx + 1 }}</span>
          <span class="rename-orig" :title="item.name">{{ item.name }}</span>
          <div class="rename-input-group">
            <InputText
              class="rename-input"
              :model-value="renames.get(item.messageId) ?? ''"
              :placeholder="nameWithoutExt(item.name)"
              @update:model-value="(v: string | undefined) => setRename(item.messageId, v ?? '')"
            />
            <span class="rename-ext">{{ extOf(item.name) }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="form-field">
      <label for="nt-script">过滤 / 命名脚本（可选）</label>
      <Select
        id="nt-script"
        v-model="scriptName"
        :options="scriptOptions"
        option-label="label"
        option-value="value"
        :placeholder="scriptPlaceholder"
        show-clear
        fluid
      />
      <span class="hint">脚本可实现按条件跳过文件、自定义文件名与任务钩子，到「脚本」页编写</span>
    </div>

    <div class="options-row">
      <div class="opt">
        <Checkbox v-model="group" input-id="nt-opt-group" binary />
        <label for="nt-opt-group">下载整组相册</label>
      </div>
      <div class="opt">
        <Checkbox v-model="rewriteExt" input-id="nt-opt-ext" binary />
        <label for="nt-opt-ext">按 MIME 修正扩展名</label>
      </div>
    </div>

    <p class="hint">同名文件将自动追加编号保存，避免覆盖已有文件。</p>

    <div class="form-field">
      <label>已下载清单</label>
      <div class="manifest-actions">
        <Button label="导入本地清单" icon="pi pi-folder-open" severity="secondary" outlined :disabled="creating" @click="importManifest" />
        <Button v-if="selection" label="新建清单" icon="pi pi-plus" severity="secondary" outlined :disabled="creating" @click="newManifest" />
        <Button v-if="selectedManifest" label="解除清单" severity="secondary" text :disabled="creating" @click="setManifest(null)" />
      </div>
      <span class="manifest-hint">磁盘使用率上限为 {{ settings.settings.diskUsageLimit }}%。达到上限时自动停止并完成本次任务。未选清单时，已下载项自动记录到下载目录中的清单，清理空间后可导入继续。</span>
      <template v-if="selectedManifest">
        <div class="manifest-card">
          <div class="manifest-card-title">
            <i class="pi pi-list" />
            <span>{{ selectedManifest.info.header.title }}</span>
            <Tag :value="`已记录 ${selectedManifest.info.done} / ${selectedManifest.info.total} 项`" severity="info" />
          </div>
          <span class="manifest-hint">下载时将自动跳过清单已记录项，下载成功后自动追加至清单。</span>
          <Message v-if="selectedManifest.info.ignored" severity="warn" :closable="false" class="manifest-warn">
            旧清单中 {{ selectedManifest.info.ignored }} 项无效记录已忽略。
          </Message>
        </div>
      </template>
    </div>

    <div v-if="previewLoading" class="preview-status loading">
      <i class="pi pi-spin pi-spinner" />
      <span>正在比对本地已有文件与清单…</span>
    </div>
    <Message v-if="previewError" severity="error" :closable="false" class="preview-msg">{{ previewError }}</Message>
    <template v-if="preview">
      <div class="preview-summary-card">
        <div class="preview-stat-item">
          <span class="stat-label">候选</span>
          <span class="stat-value">{{ preview.total }}</span>
        </div>
        <div v-if="preview.manifestSkipped" class="preview-stat-item muted">
          <span class="stat-label">清单跳过</span>
          <span class="stat-value">-{{ preview.manifestSkipped }}</span>
        </div>
        <div v-if="preview.localMatched" class="preview-stat-item muted">
          <span class="stat-label">本地已有</span>
          <span class="stat-value">-{{ preview.localMatched }}</span>
        </div>
        <div class="preview-stat-item primary">
          <span class="stat-label">预计下载</span>
          <span class="stat-value highlight">{{ preview.pending }}</span>
        </div>
      </div>
      <DuplicateReport v-if="preview.comparison" :comparison="preview.comparison" :confirmed-ids="preview.manifestMessageIds" />
      <span v-if="group" class="hint">选择整组相册时将在下载过程中展开成员，实际数量可能增加。</span>
    </template>
    <p v-if="!selection" class="hint">链接将在任务创建后自动解析并下载。</p>

    <template #footer>
      <Button label="取消" severity="secondary" text @click="close" />
      <Button
        label="创建下载任务"
        icon="pi pi-download"
        :disabled="selection ? !selection.messageIds.length || previewLoading || !!previewError || !preview : !urlList.length"
        :loading="creating"
        @click="create"
      />
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'

import DirSelect from './DirSelect.vue'
import { Download } from '../api'
import type { DownloadPreview, MediaItem, Selection } from '../types'
import { engine } from '../../wailsjs/go/models'
import { useScriptsStore } from '../stores/scripts'
import { useSettingsStore } from '../stores/settings'
import { useAuthStore } from '../stores/auth'
import { manifestScopeMatches, type ManifestSelection } from '../utils/manifests'
import DuplicateReport from './DuplicateReport.vue'

const props = defineProps<{
  visible: boolean
  manifest?: ManifestSelection | null
  /** 选集模式：指定后隐藏链接输入，直接按对话+消息 ID 下载 */
  selection?: {
    dialogId: number
    dialogType: string
    title: string
    messageIds: number[]
    /** 选中媒体项的完整信息（用于重命名列表展示原始文件名） */
    items: MediaItem[]
  } | null
}>()
const emit = defineEmits<{
  'update:visible': [boolean]
  created: []
  'manifest-change': [ManifestSelection | null]
}>()

const scripts = useScriptsStore()
const settings = useSettingsStore()
const toast = useToast()
const auth = useAuthStore()
const selectedManifest = ref<ManifestSelection | null>(null)
function setManifest(value: ManifestSelection | null) { selectedManifest.value = value; emit('manifest-change', value) }
async function importManifest() {
 try {
  const info = await Download.chooseManifest()
  if (!info) return
  const sel = props.selection
  if (sel && !manifestScopeMatches(info,auth.userId,sel.dialogId,sel.dialogType)) throw new Error('清单不属于当前账号或当前对话')
  setManifest({ info })
 } catch(e) { toast.add({severity:'error',summary:'导入清单失败',detail:String(e),life:6000}) }
}

const urls = ref('')
const dir = ref(settings.settings.downloadDir || '')
const scriptName = ref<string | null>(null)
const group = ref(false)
const rewriteExt = ref(false)
const creating = ref(false)
const dirSelect = ref<InstanceType<typeof DirSelect> | null>(null)
const preview = ref<DownloadPreview | null>(null)
const previewLoading = ref(false), previewError = ref('')
let previewEpoch = 0

async function refreshPreview(): Promise<DownloadPreview | null> {
  const mine = ++previewEpoch, sel = props.selection, owner = auth.userId
  const targetDir = dir.value.trim(), path = selectedManifest.value?.info.path ?? ''
  preview.value = null; previewError.value = ''; previewLoading.value = false
  if (!props.visible || !sel) return null
  if (!targetDir) { previewError.value = '请选择下载保存目录'; return null }
  if (sel.items.length !== sel.messageIds.length) { previewError.value = '候选媒体信息不完整，请重新选择'; return null }
  previewLoading.value = true
  const current = () => mine === previewEpoch && props.visible && props.selection === sel && auth.userId === owner && dir.value.trim() === targetDir && (selectedManifest.value?.info.path ?? '') === path
  try {
    const result = await Download.previewDownload(targetDir, sel.dialogId, sel.dialogType, sel.items, path)
    if (!current()) return null
    preview.value = result; return result
  } catch (e) { if (current()) previewError.value = String(e); return null }
  finally { if (mine === previewEpoch) previewLoading.value = false }
}
watch([() => props.visible, () => props.selection, dir, () => selectedManifest.value?.info.path, () => auth.userId], (_v, _old, cleanup) => {
  previewEpoch++; preview.value = null; previewLoading.value = false
  const timer = setTimeout(() => { void refreshPreview() }, 250)
  cleanup(() => clearTimeout(timer))
})
async function newManifest() {
  const sel = props.selection, owner = auth.userId
  if (!sel) return
  try {
    const folder = await Download.selectDirectory('manifest', '')
    if (!folder || owner !== auth.userId || props.selection !== sel) return
    const info = await Download.newDownloadManifest(folder, sel.dialogId, sel.dialogType, sel.title)
    if (owner !== auth.userId || props.selection !== sel) return
    setManifest({ info })
  } catch (e) { toast.add({ severity: 'error', summary: '新建清单失败', detail: String(e), life: 6000 }) }
}


watch(() => props.visible, (v) => {
  if (v) {
    renames.value = new Map()
    renameExpanded.value = false
    selectedManifest.value = props.manifest ? { ...props.manifest } : null
    if (!dir.value) {
      dir.value = settings.settings.downloadDir || ''
    }
    group.value = false
    dirSelect.value?.reload?.()
  }
})

// ---- 文件重命名 ----
const visibleRenameItems = computed(() => props.selection?.items?.slice(0, 100) ?? [])
/** messageId → 自定义文件名（不含扩展名），空串/不存在 = 使用默认命名 */
const renames = ref(new Map<number, string>())
const renameExpanded = ref(false)

const renameCount = computed(() => {
  let n = 0
  for (const v of renames.value.values()) {
    if (v.trim()) n++
  }
  return n
})

/** 提取文件名主体（去扩展名） */
function nameWithoutExt(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  return dot > 0 ? fileName.slice(0, dot) : fileName
}

/** 提取扩展名（含点，如 .mp4） */
function extOf(fileName: string): string {
  const dot = fileName.lastIndexOf('.')
  return dot > 0 ? fileName.slice(dot) : ''
}

function setRename(messageId: number, value: string) {
  const next = new Map(renames.value)
  const trimmed = value.trim()
  if (trimmed) {
    next.set(messageId, trimmed)
  } else {
    next.delete(messageId)
  }
  renames.value = next
}

/** 将 renames Map 转为后端期望的 Record 格式 */
function renamesToRecord(): Record<number, string> | undefined {
  if (!renames.value.size) return undefined
  const obj: Record<number, string> = {}
  for (const [k, v] of renames.value) {
    if (v.trim()) obj[k] = v.trim()
  }
  return Object.keys(obj).length ? obj : undefined
}

const urlList = computed(() =>
  urls.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean),
)

const scriptOptions = computed(() =>
  scripts.enabled.map((s) => ({ label: s.name, value: s.name })),
)

const scriptPlaceholder = computed(() =>
  scriptOptions.value.length ? '不使用脚本' : '无已启用脚本（到脚本页启用）',
)

function close() {
  renames.value = new Map()
  renameExpanded.value = false
  emit('update:visible', false)
}

async function create() {
  creating.value = true
  try {
    const sel = props.selection
    const compared = sel ? await refreshPreview() : null
    if (sel && !compared) throw new Error(previewError.value || '比对尚未完成')
    const selections: Selection[] = sel
      ? [{ dialogId: sel.dialogId, dialogType: sel.dialogType, messageIds: sel.messageIds }]
      : []
    await Download.createTask(
      engine.TaskOptions.createFrom({
        manifestPath:selectedManifest.value?.info.path ?? '',
        manifestMode:'completed',
        recordManifest:!!selectedManifest.value,
        urls: sel ? [] : urlList.value,
        selections,
        label: sel ? `${sel.title} × ${sel.messageIds.length} 个文件` : '',
        dir: dir.value,
        scriptName: scriptName.value ?? '',
        template: '{{ filenamify .FileName }}',
        rewriteExt: rewriteExt.value,
        skipSame: true,
        conflictAction: 'smart',
        conflictCheck: 'origin',
        group: group.value,
        restart: false,
        renames: renamesToRecord(),
      }),
    )
    urls.value = ''
    renames.value = new Map()
    renameExpanded.value = false
    dirSelect.value?.commit()
    emit('created')
    close()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '创建失败', detail: String(e), life: 6000 })
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.manifest-actions {display:flex;gap:8px;flex-wrap:wrap;}
.manifest-hint {font-size:12px;color:var(--p-text-muted-color);overflow-wrap:anywhere;}

.selection-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
  border-radius: 8px;
  background: var(--p-surface-100);
  font-size: 13px;
}

.app-dark .selection-summary {
  background: var(--p-surface-800);
}

.selection-summary .pi {
  color: var(--p-primary-color);
}

.options-row {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
}

.conflict-config-row {
  display: flex;
  gap: 20px;
  flex-wrap: wrap;
  align-items: center;
  margin-top: 12px;
  padding: 8px 12px;
  border-radius: 6px;
  background: var(--p-surface-50);
}

.app-dark .conflict-config-row {
  background: var(--p-surface-800);
}

.conflict-field {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.conflict-field label {
  color: var(--p-text-muted-color);
  font-size: 13px;
}

.conflict-select {
  min-width: 130px;
}

.rename-limit-tip {
  padding: 6px 10px;
  font-size: 12px;
  color: var(--p-text-muted-color);
  background: var(--p-surface-100);
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .rename-limit-tip {
  background: var(--p-surface-800);
  border-bottom-color: var(--p-surface-700);
}

.opt {
  display: flex;
  align-items: center;
  gap: 8px;
}

.opt label {
  cursor: pointer;
  font-size: 13px;
}

.form-row > .grow {
  flex: 1;
}

/* ---- 文件重命名列表 ---- */

.rename-header {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  user-select: none;
  font-size: 13px;
  font-weight: 500;
  color: var(--p-text-color);
  padding: 4px 0;
}

.rename-header .pi {
  font-size: 12px;
  color: var(--p-text-muted-color);
}

.rename-count {
  margin-left: auto;
  font-size: 12px;
  font-weight: 400;
  color: var(--p-primary-color);
}

.rename-list {
  max-height: 240px;
  overflow-y: auto;
  border: 1px solid var(--p-surface-200);
  border-radius: 6px;
  margin-top: 4px;
}

.app-dark .rename-list {
  border-color: var(--p-surface-700);
}

.rename-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  font-size: 13px;
  border-bottom: 1px solid var(--p-surface-100);
}

.app-dark .rename-row {
  border-bottom-color: var(--p-surface-800);
}

.rename-row:last-child {
  border-bottom: none;
}

.rename-idx {
  flex-shrink: 0;
  width: 20px;
  text-align: right;
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.rename-orig {
  flex-shrink: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--p-text-muted-color);
  font-size: 12px;
  max-width: 160px;
}

.rename-input-group {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 0;
  min-width: 0;
}

.rename-input {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  padding: 4px 8px;
}

.rename-ext {
  flex-shrink: 0;
  padding: 4px 6px;
  font-size: 12px;
  color: var(--p-text-muted-color);
  background: var(--p-surface-100);
  border-radius: 0 4px 4px 0;
  border: 1px solid var(--p-surface-200);
  border-left: none;
}

.app-dark .rename-ext {
  background: var(--p-surface-800);
  border-color: var(--p-surface-700);
}

.manifest-card {
  margin-top: 8px;
  padding: 10px 12px;
  background: var(--p-surface-50);
  border: 1px solid var(--p-content-border-color);
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.app-dark .manifest-card {
  background: var(--p-surface-800);
}

.manifest-card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
}

.manifest-warn {
  margin-top: 4px;
}

.preview-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--p-text-muted-color);
  margin-top: 8px;
}

.preview-summary-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 14px;
  background: var(--p-surface-50);
  border: 1px solid var(--p-content-border-color);
  border-radius: 6px;
  margin-top: 10px;
}

.app-dark .preview-summary-card {
  background: var(--p-surface-800);
}

.preview-stat-item {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 13px;
}

.preview-stat-item .stat-label {
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.preview-stat-item .stat-value {
  font-weight: 600;
}

.preview-stat-item.primary .stat-value.highlight {
  color: var(--p-primary-color);
  font-size: 15px;
}

.preview-msg {
  margin-top: 8px;
}
</style>
