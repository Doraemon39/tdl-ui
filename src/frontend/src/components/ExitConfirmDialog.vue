<template>
  <Dialog v-model:visible="store.exitRequested" modal header="退出应用" :style="{ width: '480px', maxWidth: '95vw' }" :closable="!busy" :close-on-escape="!busy">
    <div class="exit-options">
      <label v-for="option in options" :key="option.value">
        <RadioButton v-model="store.exitChoice" :value="option.value" :disabled="busy" :input-id="`exit-${option.value}`" />
        <span>{{ option.label }}<small>{{ option.hint }}</small></span>
      </label>
    </div>
    <Message v-if="store.exitChoice === 'wipe_all'" severity="warn" :closable="false">将清除本地会话、配置、任务记录和用户脚本。下载文件、自定义目录与未知文件保留；此操作无法撤销。</Message>
    <label v-if="store.exitChoice === 'wipe_all'" class="check"><Checkbox v-model="acknowledged" binary :disabled="busy" />我已了解清除范围，并确认重置应用</label>
    <label v-else class="check"><Checkbox v-model="remember" binary :disabled="busy" />记住此退出方式</label>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <template #footer>
      <Button label="取消" text severity="secondary" :disabled="busy" @click="store.exitRequested = false" />
      <Button :label="store.exitChoice === 'wipe_all' ? '清除并退出…' : '退出'" :severity="store.exitChoice === 'wipe_all' ? 'danger' : undefined" :loading="busy" :disabled="store.exitChoice === 'wipe_all' && !acknowledged" @click="quit" />
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import RadioButton from 'primevue/radiobutton'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import { useSettingsStore } from '../stores/settings'

const store = useSettingsStore()
const busy = ref(false), error = ref(''), remember = ref(false), acknowledged = ref(false)
const options = [
  { value: 'direct', label: '保留数据并退出', hint: '任务暂停，登录会话和缓存保留' },
  { value: 'clean_cache', label: '清理默认缓存后退出', hint: '仅清理默认缩略图、预览图和视频暂存分段' },
  { value: 'wipe_all', label: '清除应用数据并退出', hint: '每次单独确认，不可记住' },
]
watch([() => store.exitRequested, () => store.exitChoice], () => { error.value = ''; acknowledged.value = false; remember.value = false })
async function quit() {
  busy.value = true; error.value = ''
  try { await store.quitApp(store.exitChoice, store.exitChoice !== 'wipe_all' && remember.value) }
  catch (e) { error.value = String(e) }
  finally { busy.value = false }
}
</script>

<style scoped>
.exit-options { display: grid; gap: 16px; margin-bottom: 20px; }
.exit-options label,.check { display: flex; align-items: center; gap: 10px; cursor: pointer; }
.exit-options small { display: block; color: var(--p-text-muted-color); margin-top: 5px; }
.check { margin: 18px 0; font-size: 13px; }
</style>
