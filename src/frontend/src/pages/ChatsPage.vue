<template>
  <div class="page page-table chats-split">
    <Message v-if="!auth.loggedIn" severity="warn" class="mb-16">
      <span>尚未登录账号，请先登录后再查看对话。</span>
      <Button label="去登录" size="small" class="ml-8" @click="router.push('/login')" />
    </Message>

    <div class="split-body">
      <!-- 左面板：对话列表 -->
      <DialogListPanel :selected-id="selectedId" :selected-type="selectedType" @select="selectDialog" @export-all="exportAllFromDialog" @manifest="openManifestForDialog" />

      <!-- 右面板：媒体内容 -->
      <section class="media-panel panel-card">
        <div v-if="!selectedId" class="empty-state media-empty">
          <img class="empty-illustration" :src="emptyChats" alt="" draggable="false" />
          <p>从左侧选择一个对话，浏览并下载其中的媒体文件</p>
        </div>

        <template v-else>
          <div class="media-header">
            <div class="media-title">
              <h2 :title="selectedTitle">{{ selectedTitle }}</h2>
              <Tag :value="typeLabel(selectedType)" :severity="typeSeverity(selectedType)" />
            </div>
            <div class="media-select-bar">
              <template v-if="selectedMsgs.size">
                <span class="count-hint">已选 {{ selectedMsgs.size }} 项</span>
                <Button label="清除" size="small" severity="secondary" text @click="clearSelection" />
              </template>
              <Button
                label="全选结果"
                icon="pi pi-check-circle"
                size="small"
                severity="secondary"
                text
                :loading="scanning"
                @click="selectAllMedia()"
              />
              <Button label="已下载清单" icon="pi pi-list" size="small" severity="secondary" text :disabled="!auth.loggedIn || !chats.loaded || !validDialogType(selectedType)" @click="manifestVisible = true" />
              <Button label="下载整个对话" icon="pi pi-cloud-download" size="small" severity="secondary" text :loading="scanning" @click="downloadAllMedia" />
              <Tag v-if="activeManifest" :value="`清单 ${activeManifest.info.total} 项`" severity="info" />
              <Button
                label="下载选中"
                icon="pi pi-download"
                size="small"
                :disabled="!selectedMsgs.size"
                :badge="selectedMsgs.size ? String(selectedMsgs.size) : undefined"
                @click="downloadSelected"
              />
            </div>
          </div>

          <Message v-if="auth.loggedIn && !chats.loaded" severity="info" :closable="false">正在加载对话信息，准备就绪后将自动加载媒体。</Message>
          <div v-if="scanning" class="scan-banner">
            <i class="pi pi-spin pi-spinner" />
            <span>正在扫描历史媒体… 已发现 {{ scannedCount }} 个</span>
            <Button label="停止扫描" size="small" severity="danger" text @click="cancelScan" />
          </div>

          <MediaToolbar
            v-model:layout="layout"
            v-model:jump-month="jumpMonth"
            :applied="applied"
            @apply="applyFilters"
          />

          <div class="media-body">
            <!-- 虚拟滚动模式（大量数据时启用） -->
            <VirtualScroll
              v-if="useVirtualScroll"
              ref="virtualScrollRef"
              :items="flatMediaItems"
              :item-height="virtualScrollItemHeight"
              :container-height="virtualScrollContainerHeight"
              :has-more="hasMore"
              :loading="loading"
              :get-item-key="(item: MediaItem) => item.messageId"
              @load-more="loadMore"
              @scroll="onVirtualScroll"
              class="media-scroll"
            >
              <template #default="{ item, index }">
                <div class="virtual-media-item">
                  <div
                    class="media-card"
                    :class="{ selected: selectedMsgs.has(item.messageId) }"
                    :style="cardStyle(item)"
                    @click="toggleSelect(item, $event)"
                    @dblclick="onCardDblClick(item)"
                    @contextmenu="onCardContextMenu($event, item)"
                    @mouseenter="onCardMouseEnter(item, $event)"
                    @mouseleave="onCardMouseLeave"
                    @mousemove="onCardMouseMove"
                  >
                    <div class="thumb" @click.stop="onThumbClick(item)" @dblclick.stop>
                      <img
                        v-if="hasThumb(item)"
                        :data-src="thumbURL(item.dialogId, selectedType, item.messageId)"
                        :style="item.thumb ? { backgroundImage: `url(${item.thumb})` } : undefined"
                        class="smart-lazy-image"
                        alt=""
                        draggable="false"
                        @error="thumbFailed.add(itemKey(item))"
                        @load="onImageLoad"
                        :ref="(el) => el && observeImage(el as HTMLImageElement)"
                      />
                      <i v-else :class="kindIcon(item.kind)" class="thumb-icon" />
                      <i
                        v-if="item.kind === 'video' && hasThumb(item)"
                        class="pi pi-play-circle play-badge"
                        v-tooltip.top="'单击播放预览'"
                      />

                      <Checkbox
                        class="card-check"
                        :model-value="selectedMsgs.has(item.messageId)"
                        binary
                        @mousedown="onCheckMouseDown"
                        @click.stop
                        @update:model-value="onCheckToggle(item)"
                      />
                      <Button
                        class="card-dl"
                        icon="pi pi-download"
                        size="small"
                        rounded
                        v-tooltip.top="'下载此文件'"
                        @click.stop="downloadOne(item)"
                      />

                      <span v-if="fileStateOf(item)" class="dl-state" :class="fileStateOf(item)!.state">
                        <i v-if="fileStateOf(item)!.state === 'downloading'" class="pi pi-spin pi-spinner" />
                        <i v-else-if="fileStateOf(item)!.state === 'done'" class="pi pi-check" />
                        <i v-else class="pi pi-times" />
                        <template v-if="fileStateOf(item)!.state === 'downloading'">下载中 {{ fileStateOf(item)!.pct }}%</template>
                      </span>
                      <span v-else-if="manifestExcluded(item)" class="dl-state manifest"><i class="pi pi-list" /> 清单已记录</span>
                      <span
                        v-else-if="downloadedMap.has(item.messageId)"
                        class="dl-state done"
                        v-tooltip.top="'已下载，双击打开'"
                      >
                        <i class="pi pi-check" /> 已下载
                      </span>
                    </div>
                    <div class="card-info">
                      <span class="card-name" :title="item.name">{{ item.name }}</span>
                      <div class="card-bottom">
                        <span class="card-meta">{{ fmtSize(item.size) }} · {{ extOf(item.name) }} · {{ fmtDate(item.date) }}</span>
                        <Button
                          v-if="item.caption"
                          icon="pi pi-info-circle"
                          size="small"
                          text
                          rounded
                          severity="secondary"
                          class="card-detail-btn"
                          v-tooltip.top="'查看消息文本'"
                          @click.stop="showCaptionDetail(item)"
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </template>
            </VirtualScroll>

            <!-- 传统滚动模式（少量数据或时间线布局） -->
            <div v-else ref="scrollEl" class="media-scroll">
              <section
                v-for="g in displayGroups"
                :key="g.key"
                class="month-section"
                :ref="(el) => registerMonth(g.key, el)"
              >
                <h3 v-if="layout === 'timeline'" class="month-title">{{ g.label }}</h3>
                <div class="media-grid" :class="{ waterfall: layout !== 'grid' }" :style="gridVars">
                  <div
                    v-for="item in g.items"
                    :key="item.messageId"
                    class="media-card"
                    :class="{ selected: selectedMsgs.has(item.messageId) }"
                    :style="cardStyle(item)"
                    @click="toggleSelect(item, $event)"
                    @dblclick="onCardDblClick(item)"
                    @contextmenu="onCardContextMenu($event, item)"
                    @mouseenter="onCardMouseEnter(item, $event)"
                    @mouseleave="onCardMouseLeave"
                    @mousemove="onCardMouseMove"
                  >
                    <div class="thumb" @click.stop="onThumbClick(item)" @dblclick.stop>
                      <img
                        v-if="hasThumb(item)"
                        :data-src="thumbURL(item.dialogId, selectedType, item.messageId)"
                        :style="item.thumb ? { backgroundImage: `url(${item.thumb})` } : undefined"
                        class="smart-lazy-image"
                        alt=""
                        draggable="false"
                        @error="thumbFailed.add(itemKey(item))"
                        @load="onImageLoad"
                        :ref="(el) => el && observeImage(el as HTMLImageElement)"
                      />
                      <i v-else :class="kindIcon(item.kind)" class="thumb-icon" />
                      <i
                        v-if="item.kind === 'video' && hasThumb(item)"
                        class="pi pi-play-circle play-badge"
                        v-tooltip.top="'单击播放预览'"
                      />

                      <Checkbox
                        class="card-check"
                        :model-value="selectedMsgs.has(item.messageId)"
                        binary
                        @mousedown="onCheckMouseDown"
                        @click.stop
                        @update:model-value="onCheckToggle(item)"
                      />
                      <Button
                        class="card-dl"
                        icon="pi pi-download"
                        size="small"
                        rounded
                        v-tooltip.top="'下载此文件'"
                        @click.stop="downloadOne(item)"
                      />

                      <span v-if="fileStateOf(item)" class="dl-state" :class="fileStateOf(item)!.state">
                        <i v-if="fileStateOf(item)!.state === 'downloading'" class="pi pi-spin pi-spinner" />
                        <i v-else-if="fileStateOf(item)!.state === 'done'" class="pi pi-check" />
                        <i v-else class="pi pi-times" />
                        <template v-if="fileStateOf(item)!.state === 'downloading'">下载中 {{ fileStateOf(item)!.pct }}%</template>
                      </span>
                      <span v-else-if="manifestExcluded(item)" class="dl-state manifest"><i class="pi pi-list" /> 清单已记录</span>
                      <span
                        v-else-if="downloadedMap.has(item.messageId)"
                        class="dl-state done"
                        v-tooltip.top="'已下载，双击打开'"
                      >
                        <i class="pi pi-check" /> 已下载
                      </span>
                    </div>
                    <div class="card-info">
                      <span class="card-name" :title="item.name">{{ item.name }}</span>
                      <div class="card-bottom">
                        <span class="card-meta">{{ fmtSize(item.size) }} · {{ extOf(item.name) }} · {{ fmtDate(item.date) }}</span>
                        <Button
                          v-if="item.caption"
                          icon="pi pi-info-circle"
                          size="small"
                          text
                          rounded
                          severity="secondary"
                          class="card-detail-btn"
                          v-tooltip.top="'查看消息文本'"
                          @click.stop="showCaptionDetail(item)"
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </section>

              <div v-if="!items.length && !loading" class="empty-state small">
                <i class="pi pi-images" />
                <p>没有匹配的媒体文件</p>
              </div>

              <div ref="sentinel" class="scroll-sentinel" />

              <div class="table-status">
                <span>已加载 {{ items.length }} 条</span>
                <span v-if="loading" class="status-loading"><i class="pi pi-spin pi-spinner" /> 加载中…</span>
                <span v-else-if="failed" class="status-error">
                  上次加载失败
                  <Button label="重试" size="small" text @click="loadMore" />
                </span>
                <span v-else-if="!hasMore && items.length">没有更多了</span>
              </div>
            </div>

            <!-- 月份索引条（已加载月份锚点） -->
            <nav v-if="layout === 'timeline' && monthGroups.length && !useVirtualScroll" class="month-rail">
              <button
                v-for="g in monthGroups"
                :key="g.key"
                class="month-rail-item"
                :title="g.label"
                @click="scrollToMonth(g.key)"
              >
                {{ g.key }}
              </button>
            </nav>
          </div>
        </template>
      </section>
    </div>

    <ContextMenu ref="contextMenuRef" :model="contextMenuItems" />
    <NewTaskDialog v-model:visible="dlVisible" :selection="dlSelection" :manifest="activeManifest" @manifest-change="setActiveManifest" @created="onTaskCreated" />
    <MediaManifestDialog v-model:visible="manifestVisible" :dialog-id="selectedId ?? 0" :dialog-type="selectedType" :title="selectedTitle" :ready="auth.loggedIn && chats.loaded && validDialogType(selectedType)" :scanned-count="scannedCount" :active="activeManifest" :scan="scanForManifest" :cancel="cancelScan" @update:active="setActiveManifest" />
    <ConfirmDialog />
    <MediaPreview
      v-model:visible="previewVisible"
      v-model:index="previewIndex"
      :items="items"
      :dialog-type="selectedType"
      :has-more="hasMore"
      :downloaded-ids="downloadedIds"
      @download="downloadOne"
      @open="openDownloaded"
      @load-more="loadMore"
    />

    <!-- Caption 详情弹窗 -->
    <Dialog
      v-model:visible="captionDialogVisible"
      modal
      header="消息内容"
      :style="{ width: '480px' }"
      append-to="body"
    >
      <div class="caption-detail">{{ captionDialogText }}</div>
      <template #footer>
        <Button label="关闭" severity="secondary" text @click="captionDialogVisible = false" />
        <Button label="复制" icon="pi pi-copy" @click="copyCaptionText" />
      </template>
    </Dialog>

    <!-- 悬停 caption 预览：0.5 秒延迟后显示，跟随鼠标位置 -->
    <Teleport to="body">
      <div
        v-if="hoverCaption"
        class="caption-hover-tip"
        :style="{ left: hoverPos.x + 'px', top: hoverPos.y + 'px' }"
      >
        {{ hoverCaption }}
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import emptyChats from '../assets/illustrations/empty-chats.svg'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import ContextMenu from 'primevue/contextmenu'
import ConfirmDialog from 'primevue/confirmdialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'

import Dialog from 'primevue/dialog'
import DialogListPanel from '../components/DialogListPanel.vue'
import MediaPreview from '../components/MediaPreview.vue'
import MediaToolbar from '../components/MediaToolbar.vue'
import NewTaskDialog from '../components/NewTaskDialog.vue'
import MediaManifestDialog from '../components/MediaManifestDialog.vue'
import { manifestScopeMatches, manifestEntryMatches, type ManifestSelection } from '../utils/manifests'
import type { ManifestInfo } from '../types'
import VirtualScroll from '../components/VirtualScroll.vue'
import { EVENT_TASK, EVENT_TASK_FILE, on, thumbURL, Download } from '../api'
import type { DialogView, FileEvent, MediaItem, TaskView } from '../types'
import { peerNamespace } from '../utils/format'
import { clearMediaSnapshots, useMediaPager, type AppliedFilters } from '../composables/useMediaPager'
import { useWaterfall, gridVars, type MediaLayout } from '../composables/useWaterfall'
import { useSmartImageLoader } from '../composables/useSmartImageLoader'
import { usePerformanceMonitor } from '../utils/performance'
import { extOf, fmtDate, fmtSize, kindIcon, pad, typeLabel, typeSeverity } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { useChatsStore } from '../stores/chats'
import { useTasksStore } from '../stores/tasks'

const auth = useAuthStore()
const chats = useChatsStore()
const tasksStore = useTasksStore()
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

// 智能图片懒加载
const { observe: observeImage, preloadImages } = useSmartImageLoader({
  rootMargin: '100px',
  threshold: 0.1,
  enableCache: true,
  maxRetries: 2
})

// 性能监控
const { startTiming } = usePerformanceMonitor()

// ---- 对话选中 ----

const selectedId = ref<number | null>(null)
const selectedPeerType = ref(String(route.query.type ?? ''))
const selectedDialog = computed(() => {
  const candidates = chats.dialogs.filter(d => d.id === selectedId.value)
  return candidates.find(d => d.type === selectedPeerType.value) ?? (candidates.length === 1 ? candidates[0] : undefined)
})
// 深链兜底：store 未命中时回退路由 query 里的 type/title
const selectedType = computed(() => selectedDialog.value?.type ?? selectedPeerType.value)
const selectedTitle = computed(
  () => selectedDialog.value?.title ?? String(route.query.title ?? `对话 ${selectedId.value}`),
)

const LAST_DIALOG_KEY = 'chats.lastDialogId'
const LAST_DIALOG_CONTEXT_KEY = 'chats.lastDialogContext'
const validDialogType = (type: string) => ['private','group','supergroup','channel'].includes(type)

function selectDialog(d: DialogView) {
  if (selectedId.value === d.id && selectedPeerType.value === d.type) return
  selectedPeerType.value = d.type
  selectedId.value = d.id
  router.replace({ path: `/chats/${d.id}`, query: { type: d.type, title: d.title } })
}

watch([selectedId, selectedType, () => auth.loggedIn, () => chats.loaded], ([id, type, loggedIn, dialogsReady]) => {
  if (!loggedIn || !dialogsReady || !validDialogType(type)) { cancelScan(); switchTo(null); return }
  if (activeManifest.value && !manifestScopeMatches(activeManifest.value.info,auth.userId,id ?? 0,type)) setActiveManifest(null)
  resetThumbClick() // FUNC-001：作废上一对话未决的单击延时与双击判定
  clearSelection()
  jumpMonth.value = null
  thumbFailed.clear()
  refreshDownloaded(id)
  restoreFileStates()
  const restored = switchTo(id)
  if (restored) {
    restoring.value = true
    nextTick().then(() => {
      requestAnimationFrame(() => {
        if (scrollEl.value) {
          scrollEl.value.scrollTop = restoredScrollTop.value
        }
        restoring.value = false
      })
    })
  } else {
    loadMore()
  }
  // 持久化最后选中对话
  if (id) {
    localStorage.setItem(LAST_DIALOG_KEY, String(id))
    localStorage.setItem(LAST_DIALOG_CONTEXT_KEY, JSON.stringify({ id, type, accountId: auth.userId }))
  } else {
    localStorage.removeItem(LAST_DIALOG_KEY)
  }
})

// ---- 右面板：媒体分页（useMediaPager 内置 epoch 防乱序 + LRU 缓存） ----

const {
  items,
  hasMore,
  loading,
  failed,
  applied,
  restoredScrollTop,
  switchTo,
  saveSnapshot,
  applyFilters: pagerApplyFilters,
  jumpToDate,
  loadMore,
  loadAll,
  cancelScan,
  scanning,
  scannedCount,
} = useMediaPager({
  dialogType: () => selectedType.value,
  accountId: () => auth.userId,
  ready: () => auth.loggedIn && chats.loaded,
  onError: (e: any) =>
    toast.add({ severity: 'error', summary: '加载媒体失败', detail: String(e?.message ?? e), life: 5000 }),
  autoContinue: () => sentinelVisible && !restoring.value,
  scrollTop: () => scrollEl.value?.scrollTop ?? 0,
})

const manifestVisible = ref(false)
const activeManifest = ref<ManifestSelection | null>(null)
const manifestRecords = ref(new Map<number, ManifestInfo['entries'][number]>())
function setActiveManifest(value: ManifestSelection | null) {
  if (value && !manifestScopeMatches(value.info,auth.userId,selectedId.value ?? 0,selectedType.value)) {
    toast.add({severity:'error',summary:'清单来源不匹配',detail:'请选择当前账号和当前对话的清单',life:5000}); return
  }
  activeManifest.value = value
  manifestRecords.value = new Map((value?.info.entries ?? []).map(entry => [entry.messageId,entry]))
}
function manifestExcluded(item: MediaItem) {
  const selection = activeManifest.value
  return !!selection && manifestScopeMatches(selection.info,auth.userId,item.dialogId,selectedType.value) && manifestEntryMatches(item,manifestRecords.value.get(item.messageId),selection.mode)
}
async function scanForManifest() { return loadAll(true) }
async function openManifestForDialog(dialog: DialogView) {
  selectDialog(dialog)
  await nextTick()
  if (selectedId.value === dialog.id && selectedType.value === dialog.type) manifestVisible.value = true
}

function applyFilters(f: AppliedFilters) {
  // FE-20：重新查询从最新开始，同步清空月份跳转显示值（也使重选同一月可再次触发）
  jumpMonth.value = null
  pagerApplyFilters(f)
}

// 监听媒体项变化，触发预加载
watch(items, (newItems) => {
  if (newItems.length > 0) {
    // 延迟预加载，避免阻塞渲染
    nextTick(() => {
      preloadVisibleThumbs()
    })
  }
}, { flush: 'post' })

// ---- 无限滚动哨兵 ----

/** 哨兵提前触发距离：距可视区 600px 即开始预加载下一页 */
const SENTINEL_ROOT_MARGIN = '600px'

const sentinel = ref<HTMLElement | null>(null)
let sentinelVisible = false
let sentinelObserver: IntersectionObserver | null = null

watch(sentinel, (el, old) => {
  if (old && sentinelObserver) sentinelObserver.unobserve(old)
  if (el && sentinelObserver) sentinelObserver.observe(el)
})

// ---- 布局切换与月份分组 ----

const storedLayout = localStorage.getItem('chats.mediaLayout')
const layout = ref<MediaLayout>(storedLayout === 'waterfall' || storedLayout === 'timeline' ? storedLayout : 'grid')
watch(layout, (v) => localStorage.setItem('chats.mediaLayout', v))

// 月份跳转：以目标月的下月 1 日 0 点为 OffsetDate，后端解析为消息 ID 后走既有分页
const jumpMonth = ref<Date | null>(null)

watch(jumpMonth, (d) => {
  if (!d) return
  jumpToDate(Math.floor(new Date(d.getFullYear(), d.getMonth() + 1, 1).getTime() / 1000))
})

interface MonthGroup {
  key: string
  label: string
  items: MediaItem[]
}

// 按月份分组（Map 合并，防止跨页乱序产生重复 key）
const monthGroups = computed<MonthGroup[]>(() => {
  const map = new Map<string, MonthGroup>()
  const out: MonthGroup[] = []
  for (const it of items.value) {
    const d = new Date(it.date * 1000)
    const key = `${d.getFullYear()}-${pad(d.getMonth() + 1)}`
    let g = map.get(key)
    if (!g) {
      g = { key, label: `${d.getFullYear()} 年 ${d.getMonth() + 1} 月`, items: [] }
      map.set(key, g)
      out.push(g)
    }
    g.items.push(it)
  }
  return out
})

const displayGroups = computed<MonthGroup[]>(() =>
  layout.value === 'timeline' ? monthGroups.value : [{ key: 'all', label: '', items: items.value }],
)

// ---- 虚拟滚动配置 ----

/** 虚拟滚动启用阈值：超过此数量的媒体项时启用虚拟滚动 */
const VIRTUAL_SCROLL_THRESHOLD = 200

/** 是否启用虚拟滚动（仅在网格布局且数据量大时启用） */
const useVirtualScroll = computed(() => 
  layout.value === 'grid' && items.value.length > VIRTUAL_SCROLL_THRESHOLD
)

/** 扁平化的媒体项列表（虚拟滚动使用） */
const flatMediaItems = computed(() => items.value)

/** 虚拟滚动项高度（根据布局动态计算） */
const virtualScrollItemHeight = computed(() => {
  switch (layout.value) {
    case 'grid': return 280 // 网格布局估算高度
    case 'waterfall': return 320 // 瀑布流估算高度
    case 'timeline': return 400 // 时间线布局估算高度
    default: return 280
  }
})

/** 虚拟滚动容器高度 */
const virtualScrollContainerHeight = ref(600)

/** VirtualScroll 组件引用 */
const virtualScrollRef = ref<any>()

/** 虚拟滚动事件处理 */
function onVirtualScroll(scrollTop: number) {
  // 可以在这里添加滚动位置记录、性能监控等逻辑
  // 目前 VirtualScroll 组件内部已处理加载更多触发
}

// 监听容器大小变化，动态调整虚拟滚动容器高度
function updateVirtualScrollContainerHeight() {
  if (scrollEl.value) {
    const rect = scrollEl.value.getBoundingClientRect()
    virtualScrollContainerHeight.value = Math.max(400, rect.height)
  }
}

const monthEls = new Map<string, HTMLElement>()

function registerMonth(key: string, el: unknown) {
  if (el) monthEls.set(key, el as HTMLElement)
  else monthEls.delete(key)
}

function scrollToMonth(key: string) {
  monthEls.get(key)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// ---- 瀑布流：按纵横比计算 grid 跨行 ----

const scrollEl = ref<HTMLElement | null>(null)
const restoring = ref(false)
const { cardStyle } = useWaterfall(scrollEl, layout)

// ---- HTTP 缩略图（磁盘缓存 + 浏览器缓存接管并发） ----

const thumbFailed = reactive(new Set<string>())

function itemKey(it: MediaItem) {
  return `${it.dialogId}:${it.messageId}`
}

function hasThumb(it: MediaItem) {
  if (thumbFailed.has(itemKey(it))) return false
  return it.kind === 'photo' || it.kind === 'video' || !!it.thumb
}

// 图片加载完成处理
function onImageLoad(event: Event) {
  const img = event.target as HTMLImageElement
  img.classList.add('loaded')
}

// 预加载可见区域的缩略图
function preloadVisibleThumbs() {
  if (!items.value.length || !selectedId.value) return
  
  const endTiming = startTiming('preload-visible-thumbs')
  
  // 预加载前20个缩略图
  const visibleItems = items.value.slice(0, 20)
  const thumbUrls = visibleItems
    .filter(item => hasThumb(item))
    .map(item => thumbURL(item.dialogId, selectedType.value, item.messageId))
  
  if (thumbUrls.length > 0) {
    preloadImages(thumbUrls).finally(() => {
      endTiming()
    })
  } else {
    endTiming()
  }
}

// ---- Lightbox 预览 ----

const previewVisible = ref(false)
const previewIndex = ref(0)

function openPreview(it: MediaItem) {
  const i = items.value.findIndex((x) => x.messageId === it.messageId)
  if (i < 0) return
  previewIndex.value = i
  previewVisible.value = true
}

// ---- 悬停 caption 预览（0.5 秒延迟，类似电视平台预览效果） ----

const HOVER_DELAY_MS = 500
const hoverCaption = ref('')
const hoverPos = ref({ x: 0, y: 0 })
let hoverTimer: ReturnType<typeof setTimeout> | null = null
let hoverMsgId = 0

function onCardMouseEnter(it: MediaItem, e: MouseEvent) {
  if (!it.caption) return
  hoverMsgId = it.messageId
  hoverPos.value = { x: e.clientX + 12, y: e.clientY + 12 }
  if (hoverTimer) clearTimeout(hoverTimer)
  hoverTimer = setTimeout(() => {
    if (hoverMsgId === it.messageId) {
      hoverCaption.value = it.caption
    }
  }, HOVER_DELAY_MS)
}

function onCardMouseMove(e: MouseEvent) {
  if (hoverCaption.value) {
    hoverPos.value = { x: e.clientX + 12, y: e.clientY + 12 }
  }
}

function onCardMouseLeave() {
  if (hoverTimer) {
    clearTimeout(hoverTimer)
    hoverTimer = null
  }
  hoverCaption.value = ''
  hoverMsgId = 0
}

// ---- 卡片 caption 详情弹窗 ----

const captionDialogVisible = ref(false)
const captionDialogText = ref('')

function showCaptionDetail(it: MediaItem) {
  captionDialogText.value = it.caption
  captionDialogVisible.value = true
}

async function copyCaptionText() {
  const text = captionDialogText.value
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
  toast.add({ severity: 'success', summary: '已复制', detail: '消息文本已复制到剪贴板', life: 2000 })
}

// ---- 多选与下载 ----

const selectedMsgs = ref(new Set<number>())

/** 区间选择锚点：记录最近一次普通点击的 messageId，Shift+点击时以此为起点 */
const lastClickedMsgId = ref<number | null>(null)

// PrimeVue Checkbox 的 update:model-value 不携带原生鼠标事件，无法得知 Shift 是否按下；
// 而挂在其上的 @click.stop 又会阻断到卡片的冒泡，故在 mousedown 阶段暂存原生事件，
// 交由 onCheckToggle 转交给 toggleSelect，使复选框与卡片空白区一样支持 Shift 区间选择。
let lastCheckMouseDown: MouseEvent | null = null

function onCheckMouseDown(e: MouseEvent) {
  lastCheckMouseDown = e
}

function onCheckToggle(it: MediaItem) {
  const e = lastCheckMouseDown
  lastCheckMouseDown = null
  toggleSelect(it, e ?? undefined)
}
const dlVisible = ref(false)
const dlSelection = ref<{
  dialogId: number
  dialogType: string
  title: string
  messageIds: number[]
  /** 选中媒体项的完整信息（用于重命名列表展示原始文件名） */
  items: MediaItem[]
} | null>(null)

function toggleSelect(it: MediaItem, event?: MouseEvent) {
  // Shift+点击：以锚点为起点，对当前已加载项做连续区间选择（仅追加，不清空已有选择）
  if (event?.shiftKey && lastClickedMsgId.value !== null) {
    const anchorIdx = items.value.findIndex((x) => x.messageId === lastClickedMsgId.value)
    const clickIdx = items.value.findIndex((x) => x.messageId === it.messageId)
    if (anchorIdx >= 0 && clickIdx >= 0) {
      const [from, to] = anchorIdx < clickIdx ? [anchorIdx, clickIdx] : [clickIdx, anchorIdx]
      const next = new Set(selectedMsgs.value)
      for (let i = from; i <= to; i++) {
        next.add(items.value[i].messageId)
      }
      selectedMsgs.value = next
      lastClickedMsgId.value = it.messageId
      return
    }
  }
  const next = new Set(selectedMsgs.value)
  if (next.has(it.messageId)) {
    next.delete(it.messageId)
  } else {
    next.add(it.messageId)
  }
  selectedMsgs.value = next
  lastClickedMsgId.value = it.messageId
}

const contextMenuRef = ref()
const activeContextItem = ref<MediaItem | null>(null)

function onCardContextMenu(event: MouseEvent, item: MediaItem) {
  event.preventDefault()
  activeContextItem.value = item
  if (!selectedMsgs.value.has(item.messageId)) {
    selectedMsgs.value = new Set([item.messageId])
    lastClickedMsgId.value = item.messageId
  }
  contextMenuRef.value?.show(event)
}

const contextMenuItems = computed(() => [
  {
    label: selectedMsgs.value.size > 0 ? `下载选中项 (${selectedMsgs.value.size})` : '下载选中项',
    icon: 'pi pi-download',
    disabled: selectedMsgs.value.size === 0,
    command: () => downloadSelected(),
  },
  {
    label: '全选当前筛选结果',
    icon: 'pi pi-check-circle',
    command: () => selectAllMedia(),
  },
  {
    label: '下载全部媒体',
    icon: 'pi pi-cloud-download',
    command: () => downloadAllMedia(),
  },
  {
    separator: true,
  },
  {
    label: '选择当前已加载项',
    icon: 'pi pi-check-square',
    disabled: items.value.length === 0,
    command: () => selectAllLoaded(),
  },
  {
    label: '清除选择',
    icon: 'pi pi-times',
    disabled: selectedMsgs.value.size === 0,
    command: () => clearSelection(),
  },
])

async function selectAllMedia(unfiltered = false): Promise<boolean> {
  if (scanning.value) return false
  const dialogId = selectedId.value
  const dialogType = selectedType.value
  const owner = auth.userId
  const result = await loadAll(unfiltered)
  if (result.status === 'limit') {
    toast.add({ severity: 'warn', summary: '已达到单次扫描上限', detail: '已加载 100000 项，可下载已加载项；完整扫描未完成。', life: 6000 })
  }
  if (result.status !== 'complete' || selectedId.value !== dialogId || selectedType.value !== dialogType || auth.userId !== owner) return false
  selectedMsgs.value = new Set(result.items.map(it => it.messageId))
  toast.add({ severity: 'success', summary: '扫描完成', detail: `选中 ${result.items.length} 个符合条件的媒体文件`, life: 3000 })
  return true
}

async function downloadAllMedia() {
  if (!(await selectAllMedia(true))) return
  if (selectedMsgs.value.size) downloadSelected()
  else toast.add({ severity: 'info', summary: '未找到媒体文件', life: 3000 })
}

async function exportAllFromDialog(d: DialogView) {
  if (scanning.value) return
  selectDialog(d)
  await nextTick()
  if (selectedId.value !== d.id) return
  await downloadAllMedia()
}

function selectAllLoaded() {
  selectedMsgs.value = new Set(items.value.map((it) => it.messageId))
}

function clearSelection() {
  selectedMsgs.value = new Set()
  lastClickedMsgId.value = null
}

function openDownload(messageIds: number[]) {
  if (!selectedId.value || !messageIds.length) return
  const idSet = new Set(messageIds)
  dlSelection.value = {
    dialogId: selectedId.value,
    dialogType: selectedType.value,
    title: selectedTitle.value,
    messageIds,
    items: items.value.filter((it) => idSet.has(it.messageId)),
  }
  dlVisible.value = true
}

function downloadOne(it: MediaItem) { openDownload([it.messageId]) }
function downloadSelected() { openDownload([...selectedMsgs.value]) }
function onTaskCreated() {
  clearSelection()
  toast.add({ severity: 'success', summary: '下载任务已创建', detail: '可到「下载」页查看进度', life: 3000 })
}

// ---- 已下载标记与双击打开 ----

/** 当前对话已下载消息：messageId → 本地文件路径 */
const downloadedMap = reactive(new Map<number, string>())
const downloadedIds = computed(() => new Set(downloadedMap.keys()))

/** 切对话时重拉已下载列表（失败静默，不阻塞媒体加载） */
async function refreshDownloaded(id: number | null) {
  downloadedMap.clear()
  if (!id || !auth.loggedIn) return
  try {
    const typ = selectedType.value
    const owner = auth.userId
    const list = await Download.listDownloadedMessages(id,typ)
    if (selectedId.value !== id || selectedType.value !== typ || auth.userId !== owner) return // 返回时已切走，丢弃防串对话
    for (const f of list ?? []) downloadedMap.set(f.messageId, f.path)
  } catch {
    /* 非 Wails 环境或查询失败静默 */
  }
}

/** 双击已下载卡片：调系统默认程序打开文件 */
async function openDownloaded(it: MediaItem) {
  if (!downloadedMap.has(it.messageId)) return
  try {
    await Download.openDownloadedFile(it.dialogId,it.messageId,selectedType.value)
  } catch (e: any) {
    // 磁盘文件已被删：同步摘掉标记，避免持续误导
    downloadedMap.delete(it.messageId)
    toast.add({ severity: 'warn', summary: '打开文件失败', detail: String(e?.message ?? e), life: 5000 })
  }
}

function onCardDblClick(it: MediaItem) {
  // 卡片单击是 toggleSelect，双击两次 toggle 净效果为零，仅需处理打开
  openDownloaded(it)
}

// 缩略图区域：单击开预览会遮住第二击，对已下载项用延时区分单击/双击
const DBLCLICK_DELAY_MS = 250
let thumbClickTimer: ReturnType<typeof setTimeout> | null = null
let thumbClickMsgId = 0

// FUNC-001：单双击状态的语义边界是"当前对话"，切换对话/登出/卸载都必须复位，
// 否则旧对话的延时预览会在新对话上下文中触发，msgId 撞车还会误判双击。
function resetThumbClick() {
  if (thumbClickTimer) clearTimeout(thumbClickTimer)
  thumbClickTimer = null
  thumbClickMsgId = 0
}

function onThumbClick(it: MediaItem) {
  if (!downloadedMap.has(it.messageId)) {
    openPreview(it)
    return
  }
  if (thumbClickTimer && thumbClickMsgId === it.messageId) {
    clearTimeout(thumbClickTimer)
    thumbClickTimer = null
    openDownloaded(it)
    return
  }
  if (thumbClickTimer) clearTimeout(thumbClickTimer)
  thumbClickMsgId = it.messageId
  thumbClickTimer = setTimeout(() => {
    thumbClickTimer = null
    openPreview(it)
  }, DBLCLICK_DELAY_MS)
}

// ---- 下载状态回填（task:file 事件 → 卡片角标） ----

/** 完成角标展示时长，到期后从集合移除 */
const DONE_BADGE_TTL_MS = 5000

const fileStates = reactive(new Map<string, { state: string; pct: number }>())
let offTaskFile: (() => void) | null = null
let offTaskState: (() => void) | null = null

function fileStateOf(it: MediaItem) {
  return fileStates.get(itemKey(it))
}

/** 判断当前对话某条消息是否正在下载中（角标状态优先，tasks store 兜底） */
function isDownloading(messageId: number): boolean {
  if (!selectedId.value) return false
  if (fileStates.get(`${peerNamespace(selectedType.value)}:${selectedId.value}:${messageId}`)?.state === 'downloading') return true
  for (const byFile of Object.values(tasksStore.files)) {
    for (const ev of Object.values(byFile)) {
      if (ev.accountId === auth.userId && peerNamespace(ev.dialogType ?? '') === peerNamespace(selectedType.value) && ev.dialogId === selectedId.value && ev.messageId === messageId && ev.state === 'downloading') return true
    }
  }
  return false
}

/** 从 tasks store 回填当前对话进行中的下载角标（路由切走再回来后恢复） */
function restoreFileStates() {
  fileStates.clear()
  if (!selectedId.value) return
  for (const byFile of Object.values(tasksStore.files)) {
    for (const ev of Object.values(byFile)) {
      if (ev.accountId !== auth.userId || peerNamespace(ev.dialogType ?? '') !== peerNamespace(selectedType.value) || ev.dialogId !== selectedId.value || ev.state !== 'downloading') continue
      const pct = ev.total > 0 ? Math.round((ev.downloaded / ev.total) * 100) : 0
      fileStates.set(`${peerNamespace(ev.dialogType ?? '')}:${ev.dialogId}:${ev.messageId}`, { state: ev.state, pct })
    }
  }
}

// ---- 生命周期 ----

onMounted(() => {
  offTaskState = on<TaskView>(EVENT_TASK, async task => {
    if (!['done','failed','paused','canceled'].includes(task.status) || !activeManifest.value) return
    const path = activeManifest.value.info.path, owner = auth.userId, id = selectedId.value, typ = selectedType.value
    try {
      const info = await Download.readManifest(path)
      if (owner === auth.userId && id === selectedId.value && typ === selectedType.value && activeManifest.value?.info.path === path) setActiveManifest({info})
    } catch { /* explicit manifest controls report read failures */ }
  })
  sentinelObserver = new IntersectionObserver(
    (entries) => {
      sentinelVisible = entries[0]?.isIntersecting ?? false
      if (sentinelVisible) loadMore()
    },
    { rootMargin: SENTINEL_ROOT_MARGIN },
  )
  if (sentinel.value) sentinelObserver.observe(sentinel.value)

  // 初始化虚拟滚动容器高度
  updateVirtualScrollContainerHeight()
  
  // 监听窗口大小变化，动态调整容器高度
  window.addEventListener('resize', updateVirtualScrollContainerHeight)

  offTaskFile = on<FileEvent>(EVENT_TASK_FILE, (ev) => {
    if (ev.accountId !== auth.userId) return
    if (!ev.dialogId || !ev.messageId) return
    const key = `${peerNamespace(ev.dialogType ?? '')}:${ev.dialogId}:${ev.messageId}`
    if (ev.state === 'skipped') { fileStates.delete(key); return }
    if (ev.state === 'done' && ev.manifestPath && activeManifest.value && ev.manifestPath.toLowerCase() === activeManifest.value.info.path.toLowerCase()) {
      if (ev.manifestError) {
        toast.add({severity:'error',summary:'文件已保存，清单未更新',detail:ev.manifestError,life:7000})
      } else if (ev.mediaId && manifestScopeMatches(activeManifest.value.info,auth.userId,ev.dialogId,ev.dialogType ?? '')) {
        const info = activeManifest.value.info
        const old = manifestRecords.value.get(ev.messageId)
        if (!old || old.state !== 'done') info.done++
        const record = {...old,messageId:ev.messageId,mediaId:ev.mediaId,name:ev.originalName ?? old?.name ?? ev.name,size:ev.total,date:ev.date ?? old?.date ?? 0,state:'done',savedName:ev.name,evidence:'source'}
        manifestRecords.value.set(ev.messageId,record)
        if (old) Object.assign(old,record)
        else { info.entries.push(record); info.total++ }
      }
    }
    const pct = ev.total > 0 ? Math.round((ev.downloaded / ev.total) * 100) : 0
    fileStates.set(key, { state: ev.state, pct })
    if (ev.state === 'done') {
      // 完成即刻登记"已下载"，角标短暂展示后自然退化为常驻 chip
      if (ev.accountId === auth.userId && peerNamespace(ev.dialogType ?? '') === peerNamespace(selectedType.value) && ev.dialogId === selectedId.value && ev.path) {
        downloadedMap.set(ev.messageId, ev.path)
      }
      // 完成角标短暂展示后移除，避免长会话下集合无限增长
      setTimeout(() => {
        if (fileStates.get(key)?.state === 'done') fileStates.delete(key)
      }, DONE_BADGE_TTL_MS)
    }
  })

  restoreSelection()

})

function restoreSelection() {
  if (!auth.loggedIn || selectedId.value) return
  const pid = Number(route.params.id)
  if (Number.isSafeInteger(pid) && pid > 0) {
    selectedPeerType.value = String(route.query.type ?? '')
    selectedId.value = pid
    return
  }
  try {
    const stored = JSON.parse(localStorage.getItem(LAST_DIALOG_CONTEXT_KEY) ?? 'null')
    if (stored && typeof stored.accountId === 'number' && stored.accountId !== auth.userId) {
      localStorage.removeItem(LAST_DIALOG_KEY);localStorage.removeItem(LAST_DIALOG_CONTEXT_KEY);return
    }
    if (stored?.accountId === auth.userId && Number.isSafeInteger(stored.id) && stored.id > 0 && validDialogType(stored.type)) {
      selectedPeerType.value = stored.type; selectedId.value = stored.id
      return
    }
  } catch { /* old or malformed local preference */ }
  if (!chats.loaded) return
  const lastId = Number(localStorage.getItem(LAST_DIALOG_KEY))
  const candidates = chats.dialogs.filter(d => d.id === lastId)
  if (candidates.length === 1) selectDialog(candidates[0])
}
watch(() => [auth.loggedIn, chats.loaded], restoreSelection)

// 深链与历史前进后退：/chats/:id 变化时同步选中
//（selectDialog 的 router.replace 因 id 相同不会回环触发）
watch(
  () => [route.params.id, route.query.type],
  ([v, typ]) => {
    const pid = Number(v)
    if (pid) { selectedPeerType.value = String(typ ?? ''); selectedId.value = pid }
  },
)

onBeforeUnmount(() => {
  cancelScan()
  saveSnapshot() // 离开页面保留当前对话浏览进度（LRU 缓存）
  sentinelObserver?.disconnect()
  offTaskFile?.()
  offTaskState?.()
  resetThumbClick()
  onCardMouseLeave() // 清理悬停计时器与提示
  
  // 清理窗口大小变化监听器
  window.removeEventListener('resize', updateVirtualScrollContainerHeight)
})

// 登出后清空选中与媒体（对话列表由 DialogListPanel 负责）
watch(
  () => auth.userId,
  (id, previous) => {
    if (previous > 0 && id !== previous) {
      resetThumbClick() // FUNC-001：登出即上下文失效，未决点击一并作废
      onCardMouseLeave() // 清理悬停状态
      cancelScan()
      clearMediaSnapshots()
      selectedId.value = null
      manifestVisible.value = false
      setActiveManifest(null)
      localStorage.removeItem(LAST_DIALOG_KEY)
      localStorage.removeItem(LAST_DIALOG_CONTEXT_KEY)
    }
  },
)
</script>

<style scoped>
.dl-state.manifest {background:var(--p-primary-color);color:var(--p-primary-contrast-color);}

.mb-16 {
  margin-bottom: 16px;
}

.ml-8 {
  margin-left: 8px;
}

.split-body {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 16px;
}

/* ---- 右面板 ---- */
.media-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.media-empty {
  margin: auto;
}

.media-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--p-surface-200);
}

.app-dark .media-header {
  border-bottom-color: var(--p-surface-700);
}

.media-title {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.media-title h2 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.media-select-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.count-hint {
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.media-body {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.media-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.media-grid {
  display: grid;
  /* 几何参数由 useWaterfall.gridVars 注入，与 JS 跨行计算同源（FE-24） */
  grid-template-columns: repeat(auto-fill, minmax(var(--grid-min-col), 1fr));
  gap: var(--grid-gap);
  padding: 16px;
}

/* 瀑布流：小行步 + 按纵横比跨行，DOM 顺序保持时间倒序 */
.media-grid.waterfall {
  grid-auto-rows: var(--grid-row);
}

.waterfall .media-card {
  display: flex;
  flex-direction: column;
}

.waterfall .thumb {
  flex: 1;
  aspect-ratio: auto;
}

/* 时间流：月份标题吸顶 */
.month-title {
  position: sticky;
  top: 0;
  z-index: 2;
  margin: 0;
  padding: 8px 16px;
  font-size: 14px;
  font-weight: 600;
  background: var(--p-surface-0);
}

.app-dark .month-title {
  background: var(--p-surface-900);
}

/* 右侧月份索引条 */
.month-rail {
  width: 72px;
  flex-shrink: 0;
  overflow-y: auto;
  border-left: 1px solid var(--p-surface-200);
  display: flex;
  flex-direction: column;
  padding: 8px 4px;
  gap: 2px;
}

.app-dark .month-rail {
  border-left-color: var(--p-surface-700);
}

.month-rail-item {
  border: none;
  background: transparent;
  color: var(--p-text-muted-color);
  font-size: 12px;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s ease, color 0.15s ease;
}

.month-rail-item:hover {
  background: var(--p-surface-100);
  color: var(--p-text-color);
}

.app-dark .month-rail-item:hover {
  background: var(--p-surface-800);
}

/* ---- 媒体卡片 ---- */
.media-card {
  /* 遮罩/徽标叠加在任意缩略图上，需固定黑白配色，豁免主题 token，集中为组件级变量 */
  --mc-overlay-fg: rgb(255 255 255 / 90%);
  --mc-overlay-solid-fg: #fff;
  --mc-overlay-bg: rgb(0 0 0 / 55%);
  --mc-overlay-shadow: rgb(0 0 0 / 50%);
  --mc-hover-shadow: rgb(0 0 0 / 8%);
  border: 1px solid var(--p-surface-200);
  border-radius: 10px;
  overflow: hidden;
  cursor: pointer;
  background: var(--p-surface-0);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.app-dark .media-card {
  border-color: var(--p-surface-700);
  background: var(--p-surface-900);
}

.media-card:hover {
  border-color: var(--p-primary-color);
  box-shadow: 0 2px 8px var(--mc-hover-shadow);
}

.media-card.selected {
  border-color: var(--p-primary-color);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--p-primary-color) 40%, transparent);
}

.thumb {
  position: relative;
  aspect-ratio: 1;
  background: var(--p-surface-100);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.app-dark .thumb {
  background: var(--p-surface-800);
}

.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  /* 内嵌模糊图作背景占位，HTTP 清晰图加载完成后自然覆盖 */
  background-size: cover;
  background-position: center;
  transition: opacity 0.2s ease;
}

/* 智能懒加载样式 */
.smart-lazy-image {
  opacity: 0;
  transition: opacity 0.3s ease;
}

.smart-lazy-image.loaded {
  opacity: 1;
}

.smart-lazy-image.error {
  opacity: 0.5;
  filter: grayscale(100%);
}

/* 虚拟滚动样式 */
.virtual-media-item {
  padding: 8px;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.virtual-media-item .media-card {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.virtual-media-item .thumb {
  flex: 1;
  min-height: 0;
}

.virtual-media-item .card-info {
  flex-shrink: 0;
  padding: 8px;
}

/* 虚拟滚动容器样式 */
:deep(.virtual-scroll-container) {
  border-radius: 8px;
}

:deep(.virtual-scroll-item) {
  box-sizing: border-box;
}

.thumb-icon {
  font-size: 2rem;
  color: var(--p-surface-400);
}

.play-badge {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  font-size: 2rem;
  color: var(--mc-overlay-fg);
  text-shadow: 0 1px 4px var(--mc-overlay-shadow);
  /* 受悬停以弹出播放提示；单击仍冒泡到 .thumb 走预览逻辑 */
  cursor: pointer;
}

.card-check {
  position: absolute;
  top: 6px;
  left: 6px;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.media-card:hover .card-check,
.media-card.selected .card-check {
  opacity: 1;
}

.card-dl {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 30px;
  height: 30px;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.media-card:hover .card-dl {
  opacity: 1;
}

.dl-state {
  position: absolute;
  right: 6px;
  bottom: 6px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 11px;
  color: var(--mc-overlay-solid-fg);
  background: var(--mc-overlay-bg);
}

.dl-state.done {
  background: var(--p-green-500);
}

.dl-state.failed {
  background: var(--p-red-500);
}

.card-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
}

.card-name {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-meta {
  color: var(--p-text-muted-color);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

.card-bottom {
  display: flex;
  align-items: center;
  gap: 4px;
}

.card-detail-btn {
  flex-shrink: 0;
  width: 24px !important;
  height: 24px !important;
}

.scroll-sentinel {
  height: 1px;
}

.empty-state.small {
  padding: 48px 24px;
}

.empty-state.small .pi {
  font-size: 2rem;
  margin-bottom: 12px;
}

.status-loading {
  color: var(--p-primary-color);
}

.status-error {
  color: var(--p-red-500);
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
</style>

<!-- 悬停 caption 预览提示样式：Teleport 到 body，不能用 scoped -->
<style>
.scan-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  background: var(--p-primary-50);
  color: var(--p-primary-700);
  font-size: 13px;
  border-radius: 6px;
  margin: 8px 16px 0;
}

.app-dark .scan-banner {
  background: var(--p-primary-950);
  color: var(--p-primary-300);
}

.caption-hover-tip {
  position: fixed;
  z-index: 1200;
  max-width: 320px;
  max-height: 200px;
  overflow-y: auto;
  padding: 10px 14px;
  background: var(--p-surface-0);
  color: var(--p-text-color);
  border: 1px solid var(--p-surface-200);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgb(0 0 0 / 15%);
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  pointer-events: none;
}

.app-dark .caption-hover-tip {
  background: var(--p-surface-900);
  border-color: var(--p-surface-700);
  box-shadow: 0 4px 16px rgb(0 0 0 / 40%);
}

/* Caption 详情弹窗（Dialog append-to=body，不能用 scoped） */
.caption-detail {
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 320px;
  overflow-y: auto;
  padding: 12px;
  background: var(--p-surface-100);
  border-radius: 8px;
  font-size: 14px;
  line-height: 1.7;
  user-select: text;
}

.app-dark .caption-detail {
  background: var(--p-surface-800);
}
</style>
