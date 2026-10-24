// 媒体分页 composable（FE-11 拆分自 ChatsPage）：
// - epoch 代际防乱序（FE-01）：切换对话/重置后在途响应一律丢弃
// - dialogId 为 key 的模块级 LRU 快照缓存（FE-16）：离开页面/切换对话后返回免重拉
import { ref } from 'vue'
import { Chat } from '../api'
import type { MediaItem, MediaQuery } from '../types'

export interface AppliedFilters {
  query: string
  kinds: string[]
  exts: string[]
  minSize: number
  maxSize: number
}

export const emptyFilters = (): AppliedFilters => ({
  query: '',
  kinds: [],
  exts: [],
  minSize: 0,
  maxSize: 0,
})

interface Snapshot {
  items: MediaItem[]
  offset: number
  hasMore: boolean
  applied: AppliedFilters
  jumpOffsetDate: number
  scrollTop: number
}

// 模块级 LRU：跨组件卸载保留最近浏览对话的媒体状态
const cache = new Map<string, Snapshot>()
export function clearMediaSnapshots() { cache.clear() }
const CACHE_LIMIT = 5

const PAGE_SIZE = 50
// 连续空页（过滤后无命中）自动续拉上限，避免筛选苛刻时假死
const MAX_EMPTY_PAGES = 3

export function useMediaPager(options: {
  /** 当前对话类型（查询参数用，切换后由调用方保证与 switchTo 的 id 匹配） */
  dialogType: () => string
  accountId?: () => number
  ready?: () => boolean
  /** 加载失败回调（toast 等由调用方决定） */
  onError?: (e: unknown) => void
  /** 一轮加载结束后是否自动继续（如滚动哨兵仍在视口内） */
  autoContinue?: () => boolean
  /** 保存快照时获取当前滚动位置 */
  scrollTop?: () => number
}) {
  const items = ref<MediaItem[]>([])
  const offset = ref(0)
  const hasMore = ref(true)
  const loading = ref(false)
  const failed = ref(false)
  const applied = ref<AppliedFilters>(emptyFilters())
  const jumpOffsetDate = ref(0)
  const restoredScrollTop = ref(0)

  const scanning = ref(false)
  const scannedCount = ref(0)
  let currentKey = ''
  let currentOwner: number | undefined
  let currentId = 0
  // 请求代际：每次重置/切换递增，在途请求返回后校验不等则丢弃
  let epoch = 0

  function resetState() {
    epoch++
    scanning.value = false
    scannedCount.value = 0
    items.value = []
    offset.value = 0
    hasMore.value = true
    loading.value = false
    failed.value = false
    jumpOffsetDate.value = 0
    restoredScrollTop.value = 0
  }

  /** 保存当前对话的浏览快照（切换对话或组件卸载时调用） */
  function saveSnapshot() {
    if (!currentKey || currentOwner !== options.accountId?.() || !items.value.length || items.value.length > 2000) return
    cache.delete(currentKey)
    cache.set(currentKey, {
      items: [...items.value],
      offset: offset.value,
      hasMore: hasMore.value,
      applied: { ...applied.value, kinds: [...applied.value.kinds], exts: [...applied.value.exts] },
      jumpOffsetDate: jumpOffsetDate.value,
      scrollTop: options.scrollTop?.() ?? 0,
    })
    while (cache.size > CACHE_LIMIT) {
      cache.delete(cache.keys().next().value!)
    }
  }

  /** 切换到目标对话；命中缓存返回 true（无需 loadMore），未命中已重置状态 */
  function switchTo(dialogId: number | null): boolean {
    saveSnapshot()
    currentId = dialogId ?? 0
    currentOwner = options.accountId?.()
    currentKey = dialogId ? `${currentOwner ?? 0}:${options.dialogType()}:${dialogId}` : ''
    const snap = cache.get(currentKey)
    if (snap && dialogId) {
      // LRU touch
      cache.delete(currentKey)
      cache.set(currentKey, snap)
      epoch++
      items.value = [...snap.items]
      offset.value = snap.offset
      hasMore.value = snap.hasMore
      applied.value = { ...snap.applied, kinds: [...snap.applied.kinds], exts: [...snap.applied.exts] }
      jumpOffsetDate.value = snap.jumpOffsetDate
      scanning.value = false
      loading.value = false
      failed.value = false
      restoredScrollTop.value = snap.scrollTop
      return true
    }
    resetState()
    applied.value = emptyFilters()
    return false
  }

  /** 应用筛选条件并重新拉取（保持当前对话） */
  function applyFilters(f: AppliedFilters) {
    applied.value = { ...f, kinds: [...f.kinds], exts: [...f.exts] }
    resetState()
    void loadMore()
  }

  /** 月份跳转：以 unix 秒为起点重新拉取（保持筛选条件） */
  function jumpToDate(unixSec: number) {
    resetState()
    jumpOffsetDate.value = unixSec
    void loadMore()
  }

  function buildQuery(dialogId: number, dialogType: string): MediaQuery {
    return {
      dialogId,
      dialogType,
      offsetId: offset.value,
      offsetDate: jumpOffsetDate.value,
      limit: PAGE_SIZE,
      query: applied.value.query,
      kinds: applied.value.kinds,
      exts: applied.value.exts,
      minSize: applied.value.minSize,
      maxSize: applied.value.maxSize,
    }
  }

  function contextReady() {
    return (options.ready?.() ?? true) && ['private','group','supergroup','channel'].includes(options.dialogType())
  }
  async function loadMore() {
    if (!contextReady() || loading.value || scanning.value || !hasMore.value || !currentId) return
    const myEpoch = epoch
    const owner = options.accountId?.()
    // 固定本轮查询的对话快照，避免循环中途切换对话混入两个对话的查询
    const dialogId = currentId
    const dialogType = options.dialogType()
    loading.value = true
    failed.value = false

    try {
      let emptyPages = 0
      for (;;) {
        const page = await Chat.listMedia(buildQuery(dialogId, dialogType))
        if (myEpoch !== epoch || owner !== options.accountId?.()) return // 代际已作废：丢弃过期响应
        checkCursor(offset.value, page.nextOffset)
        mergePage(page.items ?? [])
        offset.value = page.nextOffset
        hasMore.value = page.nextOffset !== 0

        if (!hasMore.value || (page.items?.length ?? 0) > 0 || ++emptyPages >= MAX_EMPTY_PAGES) {
          break
        }
      }
    } catch (e) {
      if (myEpoch !== epoch || owner !== options.accountId?.()) return
      failed.value = true
      options.onError?.(e)
    } finally {
      if (myEpoch === epoch && owner === options.accountId?.()) {
        loading.value = false
        // 哨兵仍在视口内（如首屏未铺满）则继续拉取
        if (options.autoContinue?.() && hasMore.value && !failed.value) {
          setTimeout(() => { if (myEpoch === epoch) void loadMore() }, 0)
        }
      }
    }
  }

  function checkCursor(previous: number, next: number) {
    if (!Number.isSafeInteger(next) || next < 0 || (next > 0 && previous > 0 && next >= previous)) {
      throw new Error('媒体分页游标未前进，扫描已停止，请刷新后重试')
    }
  }

  function mergePage(page: MediaItem[]) {
    const seen = new Set(items.value.map(item => item.messageId))
    for (const item of page) {
      if (item.dialogId !== currentId) throw new Error('媒体响应不属于当前对话')
      if (!seen.has(item.messageId)) { seen.add(item.messageId); items.value.push(item) }
    }
  }

  function cancelScan() {
    epoch++
    scanning.value = false
    loading.value = false
  }

  /** Scan matching media from the latest message. Only complete results may
   * trigger the automatic download dialog; cancellation keeps loaded media. */
  async function loadAll(unfiltered = false): Promise<{ status: 'complete' | 'cancelled' | 'failed' | 'limit'; items: MediaItem[] }> {
    if (!contextReady() || scanning.value || !currentId) return { status: 'cancelled', items: [] }
    if (unfiltered) { resetState(); applied.value = emptyFilters() }
    else if (jumpOffsetDate.value > 0) resetState()
    const myEpoch = ++epoch
    const dialogId = currentId
    const dialogType = options.dialogType()
    const owner = options.accountId?.()
    const valid = () => myEpoch === epoch && currentId === dialogId && owner === options.accountId?.() && dialogType === options.dialogType() && contextReady()
    scanning.value = true
    loading.value = true
    failed.value = false
    scannedCount.value = items.value.length
    try {
      while (hasMore.value) {
        const query = { ...buildQuery(dialogId, dialogType), limit: 100 }
        const page = await Chat.listMedia(query)
        if (!valid()) return { status: 'cancelled', items: [] }
        checkCursor(offset.value, page.nextOffset)
        mergePage(page.items ?? [])
        offset.value = page.nextOffset
        hasMore.value = page.nextOffset !== 0
        scannedCount.value = items.value.length
        // Keep the UI responsive and bound memory for exceptionally large chats.
        if (items.value.length >= 100000 && hasMore.value) return { status: 'limit', items: [] }
        if (hasMore.value) await new Promise(resolve => setTimeout(resolve, 150))
        if (!valid()) return { status: 'cancelled', items: [] }
      }
      return { status: 'complete', items: [...items.value] }
    } catch (e) {
      if (!valid()) return { status: 'cancelled', items: [] }
      failed.value = true
      options.onError?.(e)
      return { status: 'failed', items: [] }
    } finally {
      if (myEpoch === epoch) { scanning.value = false; loading.value = false }
    }
  }

  return {
    loadAll, cancelScan, scanning, scannedCount,
    items,
    offset,
    hasMore,
    loading,
    failed,
    applied,
    jumpOffsetDate,
    restoredScrollTop,
    switchTo,
    saveSnapshot,
    applyFilters,
    jumpToDate,
    loadMore,
  }
}
