<template>
  <aside class="dialog-panel panel-card">
    <Message v-if="chats.error" severity="error" class="panel-msg">
      <span>加载对话失败：{{ chats.error }}</span>
      <Button label="重试" size="small" class="ml-8" :loading="chats.loading" @click="refresh" />
    </Message>

    <div class="dialog-toolbar">
      <SearchBox v-model="search" placeholder="搜索对话" class="grow" />
      <Button
        icon="pi pi-refresh"
        size="small"
        severity="secondary"
        text
        rounded
        :loading="chats.loading"
        v-tooltip.top="'刷新对话列表'"
        @click="refresh"
      />
    </div>
    <div class="dialog-toolbar filters">
      <SelectButton
        v-model="typeFilter"
        :options="typeFilterOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
        size="small"
      />
      <span class="spacer" />
      <Button
        label=".*"
        size="small"
        :severity="useRegex ? 'primary' : 'secondary'"
        :outlined="!useRegex"
        v-tooltip.top="'正则表达式'"
        @click="useRegex = !useRegex"
      />
      <Button
        label="Aa"
        size="small"
        :severity="caseSensitive ? 'primary' : 'secondary'"
        :outlined="!caseSensitive"
        v-tooltip.top="'区分大小写'"
        @click="caseSensitive = !caseSensitive"
      />
      <Button
        :icon="sortBy === 'recent' ? 'pi pi-sort-amount-down' : 'pi pi-sort-alpha-down'"
        size="small"
        severity="secondary"
        text
        rounded
        v-tooltip.top="sortBy === 'recent' ? '按最近消息排序（点击切换按标题）' : '按标题排序（点击切换按最近消息）'"
        @click="sortBy = sortBy === 'recent' ? 'title' : 'recent'"
      />
    </div>
    <span v-if="regexInvalid" class="regex-error">正则表达式无效</span>

    <VirtualScroller :items="filteredDialogs" :item-size="60" class="dialog-list">
      <template #item="{ item }">
        <div
          class="dialog-row"
          :class="{ active: item.id === selectedId && item.type === selectedType }"
          role="button"
          @click="emit('select', item)"
          @contextmenu="onRowContextMenu($event, item)"
        >
          <div class="dialog-avatar" :class="isSavedMessages(item) ? 't-saved' : `t-${item.type}`">
            <i :class="isSavedMessages(item) ? 'pi pi-bookmark' : typeIcon(item.type)" />
          </div>
          <div class="dialog-main">
            <div class="dialog-line">
              <span class="dialog-title" :title="item.title">{{ item.title }}</span>
              <span class="dialog-time">{{ fmtShortTime(item.lastMessageAt) }}</span>
            </div>
            <div class="dialog-line">
              <span class="dialog-sub">{{ isSavedMessages(item) ? '我的收藏' : (item.username ? '@' + item.username : typeLabel(item.type)) }}</span>
              <Badge v-if="item.unreadCount" :value="item.unreadCount" size="small" />
            </div>
          </div>
        </div>
      </template>
    </VirtualScroller>
    <div v-if="!filteredDialogs.length" class="empty-state small">
      <template v-if="chats.loading">
        <i class="pi pi-spin pi-spinner" />
        <p>正在加载对话…</p>
      </template>
      <template v-else>
        <i class="pi pi-comments" />
        <p>{{ chats.loaded ? '没有匹配的对话' : '暂无对话' }}</p>
      </template>
    </div>
    <div class="dialog-count">{{ filteredDialogs.length }} / {{ chats.dialogs.length }} 个对话</div>
    <ContextMenu ref="contextMenuRef" :model="contextMenuItems" />
  </aside>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Badge from 'primevue/badge'
import Button from 'primevue/button'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import VirtualScroller from 'primevue/virtualscroller'
import ContextMenu from 'primevue/contextmenu'

import SearchBox from './SearchBox.vue'
import type { DialogView } from '../types'
import { fmtShortTime, typeIcon, typeLabel } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { useChatsStore } from '../stores/chats'

defineProps<{
  selectedId: number | null
  selectedType: string
}>()
const emit = defineEmits<{
  select: [DialogView]
  'export-all': [DialogView]
  manifest: [DialogView]
}>()

const contextMenuRef = ref()
const activeDialog = ref<DialogView | null>(null)

function onRowContextMenu(event: MouseEvent, item: DialogView) {
  event.preventDefault()
  activeDialog.value = item
  contextMenuRef.value?.show(event)
}

const contextMenuItems = computed(() => [
  {label:'已下载清单',icon:'pi pi-list',command:()=>{if(activeDialog.value) emit('manifest',activeDialog.value)}},
  {
    label: activeDialog.value ? `下载「${activeDialog.value.title}」全部媒体` : '下载全部媒体',
    icon: 'pi pi-cloud-download',
    command: () => {
      if (activeDialog.value) {
        emit('export-all', activeDialog.value)
      }
    },
  },
  {
    label: '浏览此对话',
    icon: 'pi pi-folder-open',
    command: () => {
      if (activeDialog.value) {
        emit('select', activeDialog.value)
      }
    },
  },
  {
    separator: true,
  },
  {
    label: '复制对话名称',
    icon: 'pi pi-copy',
    command: () => {
      if (activeDialog.value) {
        void copyText(activeDialog.value.title)
      }
    },
  },
  {
    label: '复制对话 ID',
    icon: 'pi pi-hashtag',
    command: () => {
      if (activeDialog.value) {
        void copyText(String(activeDialog.value.id))
      }
    },
  },
])

async function copyText(value: string) {
  try { await navigator.clipboard.writeText(value); toast.add({ severity: 'success', summary: '已复制', life: 2000 }) }
  catch (e) { toast.add({ severity: 'error', summary: '复制失败', detail: String(e), life: 4000 }) }
}

const auth = useAuthStore()
const chats = useChatsStore()
const toast = useToast()

const search = ref('')
const useRegex = ref(false)
const caseSensitive = ref(false)
const typeFilter = ref<'all' | 'private' | 'group' | 'channel'>('all')
const sortBy = ref<'recent' | 'title'>('recent')

function isSavedMessages(item: DialogView): boolean {
  return item.type === 'private' && item.isSelf === true
}

const typeFilterOptions = [
  { label: '全部', value: 'all' },
  { label: '私聊', value: 'private' },
  { label: '群组', value: 'group' },
  { label: '频道', value: 'channel' },
]

// 编译正则：undefined 表示非法（无副作用，供模板提示）
const regex = computed(() => {
  if (!useRegex.value || !search.value.trim()) return null
  try {
    return new RegExp(search.value.trim(), caseSensitive.value ? '' : 'i')
  } catch {
    return undefined
  }
})
const regexInvalid = computed(() => useRegex.value && search.value.trim() !== '' && regex.value === undefined)

const filteredDialogs = computed(() => {
  let list = chats.dialogs
  if (typeFilter.value !== 'all') {
    list = list.filter((d) => d.type === typeFilter.value || (typeFilter.value === 'group' && d.type === 'supergroup'))
  }

  const q = search.value.trim()
  if (q) {
    if (useRegex.value) {
      const re = regex.value
      if (re) list = list.filter((d) => re.test(d.title) || re.test(d.username ?? ''))
    } else {
      // 普通搜索同样尊重大小写开关
      const needle = caseSensitive.value ? q : q.toLowerCase()
      const norm = (s: string) => (caseSensitive.value ? s : s.toLowerCase())
      list = list.filter((d) => norm(d.title).includes(needle) || norm(d.username ?? '').includes(needle))
    }
  }

  const sorted = [...list]
  if (sortBy.value === 'recent') {
    sorted.sort((a, b) => {
      const aSaved = isSavedMessages(a) ? 1 : 0
      const bSaved = isSavedMessages(b) ? 1 : 0
      if (aSaved !== bSaved) return bSaved - aSaved
      return (b.lastMessageAt ?? 0) - (a.lastMessageAt ?? 0)
    })
  } else {
    sorted.sort((a, b) => {
      const aSaved = isSavedMessages(a) ? 1 : 0
      const bSaved = isSavedMessages(b) ? 1 : 0
      if (aSaved !== bSaved) return bSaved - aSaved
      return a.title.localeCompare(b.title, 'zh')
    })
  }
  return sorted
})

async function refresh() {
  try {
    await chats.refresh()
  } catch (e: any) {
    toast.add({ severity: 'error', summary: '加载对话失败', detail: String(e?.message ?? e), life: 5000 })
  }
}

onMounted(() => {
  if (auth.loggedIn) refresh()
})

// 登录状态变化：登录后自动加载，登出后清空（选中态/媒体由父组件负责）
watch(
  () => auth.userId,
  (v) => {
    chats.reset()
    if (v) {
      refresh()
    } else {
      chats.reset()
    }
  },
)
</script>

<style scoped>
.ml-8 {
  margin-left: 8px;
}

.w-full {
  width: 100%;
}

.panel-msg {
  margin: 12px 12px 0;
}

.dialog-panel {
  width: 320px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.dialog-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 12px 0;
}

.dialog-toolbar.filters {
	flex-wrap: wrap;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--p-surface-200);
}

.dialog-toolbar.filters :deep(.p-selectbutton) { flex: 1 0 100%; }
.dialog-toolbar.filters :deep(.p-togglebutton) { flex: 1; white-space: nowrap; }
.dialog-toolbar .grow { min-width: 0; }

.app-dark .dialog-toolbar.filters {
  border-bottom-color: var(--p-surface-700);
}

.dialog-toolbar .grow {
  flex: 1;
}

.dialog-toolbar .spacer {
  flex: 1;
}

.regex-error {
  color: var(--p-red-500);
  font-size: 12px;
  padding: 4px 12px;
}

.dialog-list {
  flex: 1;
  min-height: 0;
}

.dialog-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 60px;
  padding: 0 12px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.dialog-row:hover {
  background: var(--p-surface-100);
}

.app-dark .dialog-row:hover {
  background: var(--p-surface-800);
}

.dialog-row.active {
  background: var(--p-primary-50);
}

.app-dark .dialog-row.active {
  background: color-mix(in srgb, var(--p-primary-color) 20%, transparent);
}

.dialog-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 头像底色为固定彩色（见下 t-* 规则），前景用主题对比色 token */
  color: var(--p-primary-contrast-color);
}

.dialog-avatar.t-saved {
  background: var(--p-primary-color, var(--p-blue-500));
}

.dialog-avatar.t-private {
  background: var(--p-green-400);
}

.dialog-avatar.t-supergroup,
.dialog-avatar.t-group {
  background: var(--p-blue-400);
}

.dialog-avatar.t-channel {
  background: var(--p-orange-400);
}

.dialog-main {
  flex: 1;
  min-width: 0;
}

.dialog-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.dialog-title {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-time {
  color: var(--p-text-muted-color);
  font-size: 11px;
  white-space: nowrap;
}

.dialog-sub {
  color: var(--p-text-muted-color);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dialog-count {
  padding: 8px 12px;
  border-top: 1px solid var(--p-surface-200);
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.app-dark .dialog-count {
  border-top-color: var(--p-surface-700);
}

.empty-state.small {
  padding: 48px 24px;
}

.empty-state.small .pi {
  font-size: 2rem;
  margin-bottom: 12px;
}
</style>
