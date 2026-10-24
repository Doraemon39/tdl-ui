<template>
  <div v-if="comparison.duplicates.length" class="duplicates">
    <div class="report-title">重名核对（{{ comparison.duplicates.length }} 组）</div>
    <p class="hint">名称和大小相同也不能证明内容一致。请核对具体消息日期、内容与本地文件；无法确认的项目保留待下载。</p>
    <details v-for="g in visibleGroups" :key="g.name">
      <summary>{{ g.name }} · {{ g.entries.length }} 条消息 / {{ g.files.length }} 个本地文件</summary>
      <div v-for="entry in g.entries.slice(0, 100)" :key="entry.messageId" class="duplicate-row">
        <span>消息 {{ entry.messageId }} · {{ fmtMessageTime(entry.date) }} · {{ fmtSize(entry.size) }}</span>
        <Tag v-if="confirmedIds?.includes(entry.messageId)" value="已确认" severity="success" />
        <Button v-if="reviewable" label="核对日期与文件" size="small" text :disabled="!g.files.some(f => f.size === entry.size)" @click="$emit('review', entry, g.files)" />
      </div>
      <p v-if="g.entries.length > 100" class="hint">此组仅展示前 100 条消息；其余重名项保留待下载，可缩小扫描范围后核对。</p>
      <div v-for="file in g.files.slice(0, 100)" :key="file.path" class="local-file">{{ file.path }} · {{ fmtSize(file.size) }} · 本地修改 {{ fmtMessageTime(file.date) }}</div>
    </details>
    <div v-if="comparison.duplicates.length > pageSize" class="pagination">
      <Button label="上一页" text size="small" :disabled="page === 0" @click="page--" />
      <span>{{ page + 1 }} / {{ Math.ceil(comparison.duplicates.length / pageSize) }}</span>
      <Button label="下一页" text size="small" :disabled="(page + 1) * pageSize >= comparison.duplicates.length" @click="page++" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import type { MediaComparison, ManifestInfo } from '../types'
import { fmtMessageTime, fmtSize } from '../utils/format'
const props = defineProps<{ comparison: MediaComparison; confirmedIds?: number[]; reviewable?: boolean }>()
defineEmits<{ review: [ManifestInfo['entries'][number], MediaComparison['duplicates'][number]['files']] }>()
const pageSize = 10, page = ref(0)
watch(() => props.comparison, () => { page.value = 0 })
const visibleGroups = computed(() => props.comparison.duplicates.slice(page.value * pageSize, (page.value + 1) * pageSize))
</script>

<style scoped>
.duplicates { margin-top: 12px; font-size: 13px; }
.report-title { font-weight: 600; }
details { border: 1px solid var(--p-content-border-color); border-radius: 6px; padding: 10px; margin: 8px 0; }
summary { cursor: pointer; overflow-wrap: anywhere; }
.duplicate-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.local-file { color: var(--p-text-muted-color); overflow-wrap: anywhere; padding-top: 6px; }
.pagination { display: flex; align-items: center; justify-content: center; gap: 8px; }
</style>
