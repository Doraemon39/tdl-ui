<template>
  <div class="form-row">
    <Select
      :input-id="inputId"
      :model-value="modelValue"
      editable
      :options="options"
      :placeholder="placeholder || '请选择或输入目录'"
      class="grow"
	  :disabled="disabled"
      @update:model-value="(v: string) => emit('update:modelValue', v)"
    />
    <Button icon="pi pi-folder-open" severity="secondary" outlined :disabled="disabled" v-tooltip.top="'浏览…'" @click="browse" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Select from 'primevue/select'

import { Download, SettingsApi } from '../api'

const props = defineProps<{
  modelValue: string
  kind: string
  placeholder?: string
  inputId?: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [string]
}>()

const toast = useToast()
const options = ref<string[]>([])

async function loadOptions() {
  try {
    const dirs = await SettingsApi.recentDirs(props.kind)
    options.value = dirs ?? []
    // 若当前值为空且存在历史，默认选中最近一次
    if (!props.modelValue && options.value.length && ['download', 'logExport', 'manifest'].includes(props.kind)) {
      emit('update:modelValue', options.value[0])
    }
  } catch {
    /* 非 Wails 环境静默 */
  }
}

onMounted(loadOptions)

watch(() => props.kind, () => {
  loadOptions()
})

async function browse() {
  try {
    const picked = await Download.selectDirectory(props.kind, props.modelValue)
    if (picked) {
      emit('update:modelValue', picked)
      await loadOptions()
    }
  } catch (e: any) {
    toast.add({
      severity: 'error',
      summary: '打开目录选择框失败',
      detail: String(e?.message ?? e),
      life: 5000,
    })
  }
}

// 由调用方在确认/保存成功后调用，把手工输入的目录也进历史
async function commit() {
  if (!props.modelValue) return
  try {
    await SettingsApi.addRecentDir(props.kind, props.modelValue)
    await loadOptions()
  } catch {
    /* 静默 */
  }
}

defineExpose({ commit, reload: loadOptions })
</script>
