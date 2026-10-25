<template>
  <div class="page">
    <div class="page-header header-row">
      <div>
        <h1>下载</h1>
        <p>下载任务的实时进度与控制</p>
        <div v-if="tasks.tasks.length > 0" class="tasks-summary-bar">
          <span class="sum-item"><i class="pi pi-list" /> {{ tasks.tasks.length }} 个任务</span>
          <span class="sum-item sum-success"><i class="pi pi-check-circle" /> 成功 {{ totalFinished }}</span>
          <span v-if="totalFailed > 0" class="sum-item sum-danger"><i class="pi pi-times-circle" /> 失败 {{ totalFailed }}</span>
          <span v-if="totalSkipped > 0" class="sum-item sum-warn"><i class="pi pi-ban" /> 跳过 {{ totalSkipped }}</span>
        </div>
      </div>
      <div class="header-actions">
        <div v-if="tasks.totalSpeed > 0" class="total-speed">
          <i class="pi pi-bolt" />
          <span>{{ fmtSpeed(tasks.totalSpeed) }}</span>
        </div>
        <Button
          label="清除已完成记录"
          icon="pi pi-eraser"
          severity="secondary"
          outlined
          :disabled="!hasFinished"
          @click="onClearFinished"
        />
        <Button
          label="删除所有文件"
          icon="pi pi-trash"
          severity="danger"
          outlined
          :disabled="!tasks.tasks.length"
          @click="onDeleteAll"
        />
        <Button label="添加下载" icon="pi pi-plus" @click="showDialog = true" />
      </div>
    </div>

    <div v-if="!tasks.tasks.length" class="empty-state panel-card">
      <img class="empty-illustration" :src="emptyTasks" alt="" draggable="false" />
      <p>暂无下载任务</p>
      <p class="mt-8">点击右上角「添加下载」创建第一个任务</p>
    </div>

    <template v-else>
      <div class="filter-toolbar panel-card">
        <SearchBox
          v-model="query"
          placeholder="搜索任务（标题/链接/目录/脚本）"
          class="filter-search"
          size="small"
          :invalid="regexInvalid"
        />
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
        <span class="match-count">{{ filteredTasks.length }} / {{ tasks.tasks.length }} 个任务</span>
      </div>
      <div v-if="regexInvalid" class="regex-error">正则表达式无效（已退化为不过滤）</div>
      <div v-if="!filteredTasks.length" class="empty-state panel-card">
        <p>没有匹配的任务</p>
      </div>
    </template>

    <div v-for="t in filteredTasks" :key="t.id" class="task-card">
      <div class="row">
        <Tag :value="statusLabel(t.status)" :severity="statusSeverity(t.status)" />
        <span class="title" :title="t.label || (t.urls ?? []).join('\n')">{{ taskTitle(t) }}</span>
        <div class="task-stats-badges">
          <span class="stat-badge stat-success" title="成功下载数">
            <i class="pi pi-check" /> 成功 {{ t.finished }}
          </span>
          <span v-if="t.failed > 0" class="stat-badge stat-danger" title="失败数">
            <i class="pi pi-times" /> 失败 {{ t.failed }}
          </span>
          <span v-if="(t.skipped ?? 0) > 0" class="stat-badge stat-warn" title="已跳过数">
            <i class="pi pi-ban" /> 跳过 {{ t.skipped }}
          </span>
          <span class="stat-badge stat-total" title="总文件数">
            / 共 {{ Math.max(t.total, t.fileCount || 0, t.finished + t.failed + (t.skipped ?? 0)) }} 个
          </span>
        </div>

        <Button
          icon="pi pi-folder-open"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'打开目录'"
          @click="onOpenDir(t.id)"
        />
        <Button
          v-if="t.status === 'running' || t.status === 'queued'"
          icon="pi pi-pause"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'暂停'"
          @click="onPause(t.id)"
        />
        <Button
          v-if="t.status === 'paused'"
          icon="pi pi-play"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'恢复（断点续传）'"
          @click="onResume(t.id)"
        />
        <Button
          v-if="t.status === 'failed' || t.status === 'canceled'"
          icon="pi pi-refresh"
          severity="warn"
          text
          rounded
          v-tooltip.top="'重试（断点续传）'"
          @click="onResume(t.id)"
        />
        <Button
          v-if="hasPendingFiles(t) && (t.status === 'done' || t.status === 'failed' || t.status === 'canceled')"
          icon="pi pi-forward"
          severity="info"
          text
          rounded
          v-tooltip.top="'继续下载未完成文件'"
          @click="onResumeWithPending(t.id)"
        />
        <Button
          v-if="t.status === 'running' || t.status === 'queued' || t.status === 'paused'"
          icon="pi pi-times"
          severity="danger"
          text
          rounded
          v-tooltip.top="'取消'"
          @click="onCancel(t.id)"
        />
        <Button
          v-if="isFinal(t.status)"
          icon="pi pi-trash"
          severity="secondary"
          text
          rounded
          v-tooltip.top="'移除记录'"
          @click="onRemove(t.id)"
        />
        <Button
          v-if="isFinal(t.status)"
          :icon="expanded[t.id] ? 'pi pi-chevron-up' : 'pi pi-list'"
          severity="secondary"
          text
          rounded
                v-tooltip.top="'文件列表'"
                aria-label="文件列表"
          @click="toggleFiles(t)"
        />
      </div>

      <ProgressBar
        class="mt-8"
        :value="taskPercent(t)"
        :mode="t.status === 'running' && !t.total ? 'indeterminate' : 'determinate'"
        :show-value="false"
        style="height: 6px"
      />

      <div class="task-meta mt-8">
        <span><i class="pi pi-folder" /> {{ t.dir }}</span>
        <span v-if="t.scriptName"><i class="pi pi-code" /> {{ t.scriptName }}</span>
        <span class="meta-stat-success"><i class="pi pi-check-circle" /> 成功 {{ t.finished }}</span>
        <span v-if="t.failed" class="meta-stat-failed"><i class="pi pi-exclamation-triangle" /> 失败 {{ t.failed }}</span>
        <span v-if="t.skipped" class="meta-stat-skipped"><i class="pi pi-ban" /> 跳过 {{ t.skipped }}</span>
        <span v-if="t.fileCount"><i class="pi pi-file" /> 已登记 {{ t.fileCount }} 个文件</span>
        <span v-if="t.speed && t.status === 'running'"><i class="pi pi-bolt" /> {{ fmtSpeed(t.speed) }}</span>
        <span>{{ t.createdAt }}</span>
      </div>

      <Message v-if="t.status === 'running' && t.activity" severity="info" class="mt-8" size="small">{{ t.activity }}</Message>
      <Message v-if="t.error" severity="error" class="mt-8" size="small">{{ t.error }}</Message>
      <Message v-if="t.notice" severity="warn" class="mt-8" size="small" style="white-space: pre-wrap; overflow-wrap: anywhere">{{ t.notice }}</Message>

      <!-- 进行中的文件 -->
      <div v-for="f in tasks.filesOf(t.id)" :key="f.fileId" class="file-row">
        <i :class="f.state === 'failed' ? 'pi pi-times-circle failed' : 'pi pi-arrow-circle-down'" />
        <span class="name" :title="f.name">{{ f.name }}</span>
        <span class="size">{{ fmtSize(f.downloaded) }} / {{ fmtSize(f.total) }}</span>
        <span v-if="f.speed && f.state === 'downloading'" class="speed">{{ fmtSpeed(f.speed) }}</span>
        <ProgressBar
          :value="f.total > 0 ? Math.round((f.downloaded / f.total) * 100) : 0"
          :show-value="false"
          style="width: 120px; height: 4px"
        />
      </div>

      <!-- 已登记文件列表（展开时懒加载） -->
      <div v-if="expanded[t.id]" class="files-panel mt-8">
        <div v-if="loadingFiles[t.id]" class="files-hint">加载中…</div>
        <template v-else>
          <div v-if="!fileLists[t.id]?.length" class="files-hint">无文件记录</div>
          <template v-else>
            <div class="file-filter-toolbar">
              <div class="file-status-filter">
                <Button
                  label="全部"
                  size="small"
                  :badge="String(fileLists[t.id]?.length ?? 0)"
                  :severity="(!fileStatusFilter[t.id] || fileStatusFilter[t.id] === 'all') ? 'primary' : 'secondary'"
                  :outlined="fileStatusFilter[t.id] !== 'all' && Boolean(fileStatusFilter[t.id])"
                  @click="fileStatusFilter[t.id] = 'all'"
                />
                <Button
                  label="成功"
                  size="small"
                  :badge="String(fileCountByState(t.id, 'done'))"
                  badge-severity="success"
                  :severity="fileStatusFilter[t.id] === 'done' ? 'success' : 'secondary'"
                  :outlined="fileStatusFilter[t.id] !== 'done'"
                  @click="fileStatusFilter[t.id] = 'done'"
                />
                <Button
                  v-if="fileCountByState(t.id, 'failed') > 0"
                  label="失败"
                  size="small"
                  :badge="String(fileCountByState(t.id, 'failed'))"
                  badge-severity="danger"
                  :severity="fileStatusFilter[t.id] === 'failed' ? 'danger' : 'secondary'"
                  :outlined="fileStatusFilter[t.id] !== 'failed'"
                  @click="fileStatusFilter[t.id] = 'failed'"
                />
                <Button
                  v-if="fileCountByState(t.id, 'skipped') > 0"
                  label="已跳过"
                  size="small"
                  :badge="String(fileCountByState(t.id, 'skipped'))"
                  badge-severity="warn"
                  :severity="fileStatusFilter[t.id] === 'skipped' ? 'warn' : 'secondary'"
                  :outlined="fileStatusFilter[t.id] !== 'skipped'"
                  @click="fileStatusFilter[t.id] = 'skipped'"
                />
                <Button
                  :label="isAllSelected(t.id) ? '取消全选' : '全选'"
                  size="small"
                  severity="secondary"
                  text
                  :icon="isAllSelected(t.id) ? 'pi pi-check-square' : 'pi pi-stop'"
                  v-tooltip.top="'全选/取消全选当前筛选的文件'"
                  @click="toggleSelectAll(t.id)"
                />
              </div>
              <div class="file-search-bar">
                <SearchBox
                  v-model="fileQueries[t.id]"
                  placeholder="搜索文件名/路径/原因"
                  class="grow"
                  size="small"
                  :invalid="fileRegexInvalid(t.id)"
                />
                <Button
                  label=".*"
                  size="small"
                  :severity="fileUseRegex[t.id] ? 'primary' : 'secondary'"
                  :outlined="!fileUseRegex[t.id]"
                  v-tooltip.top="'正则表达式'"
                  @click="fileUseRegex[t.id] = !fileUseRegex[t.id]"
                />
                <Button
                  label="Aa"
                  size="small"
                  :severity="fileCaseSensitive[t.id] ? 'primary' : 'secondary'"
                  :outlined="!fileCaseSensitive[t.id]"
                  v-tooltip.top="'区分大小写'"
                  @click="fileCaseSensitive[t.id] = !fileCaseSensitive[t.id]"
                />
              </div>
            </div>
            <div v-if="fileRegexInvalid(t.id)" class="regex-error">正则表达式无效（已退化为不过滤）</div>
            <div v-if="!filteredFiles(t.id).length" class="files-hint">没有匹配的文件</div>
          </template>
          <div v-for="f in filteredFiles(t.id)" :key="f.path" class="task-file-row">
            <Checkbox v-model="selectedPaths[t.id]" :value="f.path" />
            <span class="name" :title="f.path">{{ f.name }}</span>
            <span v-if="f.reason" class="file-reason" :title="f.reason">{{ f.reason }}</span>
            <span class="size">{{ fmtSize(f.size) }}</span>
            <Tag
              :value="fileStateLabel(f.state)"
              :severity="fileStateSeverity(f.state)"
              class="state-tag"
            />
            <div class="file-actions">
              <Button
                v-if="f.state === 'failed' || f.state === 'done' || f.state === 'skipped'"
                :disabled="t.status === 'running' || t.status === 'queued'"
                icon="pi pi-refresh"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'重新下载'"
                @click="onRedownloadFile(t.id, f.path)"
              />
              <Button
                v-if="f.state !== 'skipped'"
                icon="pi pi-folder-open"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'打开所在目录'"
                @click="onRevealFile(f.path)"
              />
              <Button
                icon="pi pi-times"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'删除记录'"
                @click="onDeleteFileRecord(t.id, f.path)"
              />
              <Button
                icon="pi pi-trash"
                severity="danger"
                text
                rounded
                size="small"
                v-tooltip.top="'删除文件'"
                v-if="f.state !== 'skipped'"
                @click="onDeleteFile(t.id, f.path)"
              />
            </div>
          </div>
          <div v-if="selectedPaths[t.id]?.length" class="files-actions">
            <Button
              :label="`重新下载选中（${selectedPaths[t.id].length}）`"
              :disabled="t.status === 'running' || t.status === 'queued'"
              icon="pi pi-refresh"
              severity="primary"
              size="small"
              @click="onRedownloadSelected(t)"
            />
            <Button
              :label="`删除选中文件（${selectedPaths[t.id].length}）`"
              icon="pi pi-trash"
              severity="danger"
              size="small"
              outlined
              @click="onDeleteSelected(t)"
            />
          </div>
        </template>
      </div>
    </div>

    <NewTaskDialog v-model:visible="showDialog" @created="onCreated" />
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import ConfirmDialog from 'primevue/confirmdialog'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'

import NewTaskDialog from '../components/NewTaskDialog.vue'
import SearchBox from '../components/SearchBox.vue'
import emptyTasks from '../assets/illustrations/empty-tasks.svg'
import { Download } from '../api'
import { fmtSize, fmtSpeed } from '../utils/format'
import type { TaskFile, TaskView } from '../types'
import { useTasksStore } from '../stores/tasks'

const tasks = useTasksStore()
const toast = useToast()
const confirm = useConfirm()
const showDialog = ref(false)

// 全局总速度轮询（1s 间隔，避免高频事件推送）
let speedTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  speedTimer = setInterval(() => {
    tasks.refreshTotalSpeed()
  }, 1000)
})
onUnmounted(() => {
  if (speedTimer) {
    clearInterval(speedTimer)
    speedTimer = null
  }
})

// 全局统计指标汇总
const totalFinished = computed(() => tasks.tasks.reduce((sum, t) => sum + (t.finished || 0), 0))
const totalFailed = computed(() => tasks.tasks.reduce((sum, t) => sum + (t.failed || 0), 0))
const totalSkipped = computed(() => tasks.tasks.reduce((sum, t) => sum + (t.skipped || 0), 0))

// 任务文件列表的展开与多选状态
const expanded = reactive<Record<string, boolean>>({})
const loadingFiles = reactive<Record<string, boolean>>({})
const fileLists = reactive<Record<string, TaskFile[]>>({})
const selectedPaths = reactive<Record<string, string[]>>({})
const fileStatusFilter = reactive<Record<string, 'all' | 'done' | 'failed' | 'skipped'>>({})

function fileCountByState(taskId: string, state: string): number {
  return (fileLists[taskId] ?? []).filter((f) => f.state === state).length
}

// 任务级搜索过滤状态
const query = ref('')
const useRegex = ref(false)
const caseSensitive = ref(false)

// 文件级搜索过滤状态（按任务 ID 独立）
const fileQueries = reactive<Record<string, string>>({})
const fileUseRegex = reactive<Record<string, boolean>>({})
const fileCaseSensitive = reactive<Record<string, boolean>>({})

// 通用匹配器工厂：返回 null 表示无过滤（空查询或非法正则降级）
function buildMatcher(q: string, regex: boolean, cs: boolean): ((s: string) => boolean) | null {
  const t = q.trim()
  if (!t) return null
  if (regex) {
    try {
      const re = new RegExp(t, cs ? '' : 'i')
      return (s) => re.test(s)
    } catch {
      return null // 非法正则退化为不过滤
    }
  }
  const needle = cs ? t : t.toLowerCase()
  return (s) => (cs ? s : s.toLowerCase()).includes(needle)
}

// 正则是否非法（仅正则模式且非空查询时判定）
function isRegexInvalid(q: string, regex: boolean): boolean {
  const t = q.trim()
  if (!regex || !t) return false
  try {
    new RegExp(t)
    return false
  } catch {
    return true
  }
}

const regexInvalid = computed(() => isRegexInvalid(query.value, useRegex.value))

// taskTitle 需在 filteredTasks 之前定义，避免 setup 编译后引用未初始化绑定（TDZ）导致渲染抛错
function taskTitle(t: TaskView) {
  if (t.label) return t.label
  const urls = t.urls ?? []
  const first = urls[0] ?? ''
  return urls.length > 1 ? `${first} 等 ${urls.length} 条链接` : first
}

function taskPercent(t: TaskView) {
  if (t.status === 'done') return 100
  const total = t.total || (t.finished + t.failed + (t.skipped ?? 0))
  if (!total) return 0
  return Math.min(100, Math.round(((t.finished + t.failed + (t.skipped ?? 0)) / total) * 100))
}

const filteredTasks = computed(() => {
  const m = buildMatcher(query.value, useRegex.value, caseSensitive.value)
  if (!m) return tasks.tasks
  return tasks.tasks.filter(
    (t) => m(taskTitle(t)) || m((t.urls ?? []).join(' ')) || m(t.dir) || m(t.scriptName),
  )
})

function filteredFiles(taskId: string): TaskFile[] {
  let list = fileLists[taskId] ?? []
  const filter = fileStatusFilter[taskId] ?? 'all'
  if (filter !== 'all') {
    list = list.filter((f) => f.state === filter)
  }
  const m = buildMatcher(fileQueries[taskId] ?? '', fileUseRegex[taskId] ?? false, fileCaseSensitive[taskId] ?? false)
  if (!m) return list
  return list.filter((f) => m(f.name) || m(f.path) || (f.reason && m(f.reason)))
}

function fileRegexInvalid(taskId: string): boolean {
  return isRegexInvalid(fileQueries[taskId] ?? '', fileUseRegex[taskId] ?? false)
}

// FE-25：任务被移除或重新运行（离开终态）时清理展开/文件列表状态，避免缓慢累积与陈旧列表
watch(
  () => tasks.tasks.map((t) => `${t.id}:${t.status}`),
  () => {
    const finalById = new Map(tasks.tasks.map((t) => [t.id, isFinal(t.status)]))
    for (const key of Object.keys(expanded)) {
      if (!finalById.get(key)) {
        delete expanded[key]
        delete loadingFiles[key]
        delete fileLists[key]
        delete selectedPaths[key]
        delete fileStatusFilter[key]
        delete fileQueries[key]
        delete fileUseRegex[key]
        delete fileCaseSensitive[key]
      }
    }
  },
)

const hasFinished = computed(() => tasks.tasks.some((t) => t.status === 'done'))

async function toggleFiles(t: TaskView) {
  if (expanded[t.id]) {
    expanded[t.id] = false
    return
  }
  expanded[t.id] = true
  if (!selectedPaths[t.id]) selectedPaths[t.id] = []
  // 初始化文件级搜索框绑定值，避免 SearchBox 的 modelValue 收到 undefined
  if (fileQueries[t.id] === undefined) fileQueries[t.id] = ''
  if (fileUseRegex[t.id] === undefined) fileUseRegex[t.id] = false
  if (fileCaseSensitive[t.id] === undefined) fileCaseSensitive[t.id] = false
  loadingFiles[t.id] = true
  try {
    fileLists[t.id] = (await Download.listTaskFiles(t.id)) ?? []
  } catch (e) {
    toast.add({ severity: 'error', summary: '加载文件列表失败', detail: String(e), life: 4000 })
    expanded[t.id] = false
  } finally {
    loadingFiles[t.id] = false
  }
}

async function onOpenDir(id: string) {
  try {
    await tasks.openDir(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '打开目录失败', detail: String(e), life: 4000 })
  }
}

async function onPause(id: string) {
  try {
    await tasks.pause(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '暂停失败', detail: String(e), life: 4000 })
  }
}

async function onResume(id: string) {
  try {
    await tasks.resume(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '恢复失败', detail: String(e), life: 4000 })
  }
}

async function onCancel(id: string) {
  try {
    await tasks.cancel(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '取消失败', detail: String(e), life: 4000 })
  }
}

async function onRemove(id: string) {
  try {
    await tasks.remove(id)
  } catch (e) {
    toast.add({ severity: 'error', summary: '移除记录失败', detail: String(e), life: 4000 })
  }
}

async function onClearFinished() {
  try {
    await tasks.clearFinished()
    toast.add({ severity: 'success', summary: '已清除完成记录', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: '清除失败', detail: String(e), life: 5000 })
  }
}

function onDeleteAll() {
  confirm.require({
    header: '删除所有文件',
    message: '将停止所有未完成任务，并删除全部已登记的下载文件与任务记录，且不可恢复。确定继续？',
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '删除', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await tasks.deleteAllFiles()
        for (const k of Object.keys(expanded)) delete expanded[k]
        toast.add({ severity: 'success', summary: '已删除全部文件与记录', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

function onDeleteSelected(t: TaskView) {
  const paths = selectedPaths[t.id] ?? []
  if (!paths.length) return
  confirm.require({
    header: '删除选中文件',
    message: `将删除 ${paths.length} 个文件（记录同步移除），且不可恢复。确定继续？`,
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '删除', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await tasks.deleteFiles(t.id, paths)
        selectedPaths[t.id] = []
        if (tasks.tasks.some((x) => x.id === t.id)) {
          fileLists[t.id] = (await Download.listTaskFiles(t.id)) ?? []
        } else {
          delete expanded[t.id] // 文件全部删除，记录已被后端移除
        }
        toast.add({ severity: 'success', summary: '已删除选中文件', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

// 检查任务是否有未完成的文件（用于显示"继续下载"按钮）
function hasPendingFiles(t: TaskView): boolean {
  // 如果任务有失败计数，说明有未完成的文件
  if (t.failed > 0) return true
  // 如果任务已完成但文件列表中有未完成或失败的文件，也显示继续下载按钮
  const files = fileLists[t.id]
  if (!files) return false
  return files.some((f) => f.state === 'downloading' || f.state === 'failed')
}

// 继续下载未完成文件
async function onResumeWithPending(id: string) {
  try {
    await tasks.resumeTaskWithPending(id)
    toast.add({ severity: 'success', summary: '已继续下载未完成文件', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: '继续下载失败', detail: String(e), life: 5000 })
  }
}

// 重新下载单个文件
async function onRedownloadFile(taskId: string, filePath: string) {
  try {
    await tasks.redownloadFile(taskId, filePath)
    toast.add({ severity: 'success', summary: '已加入重新下载队列', life: 3000 })
    // 刷新文件列表
    fileLists[taskId] = (await Download.listTaskFiles(taskId)) ?? []
  } catch (e) {
    toast.add({ severity: 'error', summary: '重新下载失败', detail: String(e), life: 5000 })
  }
}

// 批量重新下载选中的文件（如批量重新下载跳过或失败的文件）
async function onRedownloadSelected(t: TaskView) {
  const paths = selectedPaths[t.id] ?? []
  if (!paths.length) return
  try {
    await tasks.redownloadFiles(t.id, paths)
    selectedPaths[t.id] = []
    fileLists[t.id] = (await Download.listTaskFiles(t.id)) ?? []
    toast.add({ severity: 'success', summary: `已创建重下任务（${paths.length} 个文件）`, life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: '重新下载失败', detail: String(e), life: 5000 })
  }
}

function isAllSelected(taskId: string): boolean {
  const current = filteredFiles(taskId)
  if (!current.length) return false
  const selected = new Set(selectedPaths[taskId] ?? [])
  return current.every((f) => selected.has(f.path))
}

function toggleSelectAll(taskId: string) {
  const current = filteredFiles(taskId)
  if (!current.length) return
  if (!selectedPaths[taskId]) selectedPaths[taskId] = []
  if (isAllSelected(taskId)) {
    const currentPaths = new Set(current.map((f) => f.path))
    selectedPaths[taskId] = selectedPaths[taskId].filter((p) => !currentPaths.has(p))
  } else {
    const set = new Set(selectedPaths[taskId])
    for (const f of current) {
      set.add(f.path)
    }
    selectedPaths[taskId] = Array.from(set)
  }
}

// 打开文件所在目录并选中
async function onRevealFile(filePath: string) {
  try {
    await Download.revealFile(filePath)
  } catch (e) {
    toast.add({ severity: 'error', summary: '打开目录失败', detail: String(e), life: 4000 })
  }
}

// 删除文件记录（保留磁盘文件）
async function onDeleteFileRecord(taskId: string, filePath: string) {
  try {
    await tasks.deleteFileRecord(taskId, filePath)
    toast.add({ severity: 'success', summary: '已删除记录', life: 3000 })
    fileLists[taskId] = (await Download.listTaskFiles(taskId)) ?? []
  } catch (e) {
    toast.add({ severity: 'error', summary: '删除记录失败', detail: String(e), life: 5000 })
  }
}

// 删除文件（含磁盘文件）
function onDeleteFile(taskId: string, filePath: string) {
  confirm.require({
    header: '删除文件',
    message: '将删除该文件的磁盘副本与记录，且不可恢复。确定继续？',
    icon: 'pi pi-exclamation-triangle',
    acceptProps: { label: '删除', severity: 'danger' },
    rejectProps: { label: '取消', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await tasks.deleteFiles(taskId, [filePath])
        toast.add({ severity: 'success', summary: '已删除文件', life: 3000 })
        fileLists[taskId] = (await Download.listTaskFiles(taskId)) ?? []
      } catch (e) {
        toast.add({ severity: 'error', summary: '删除失败', detail: String(e), life: 5000 })
      }
    },
  })
}

const fileStateText: Record<string, string> = {
  downloading: '未完成',
  done: '已完成',
  failed: '失败',
  skipped: '已跳过',
}

function fileStateLabel(s: string) {
  return fileStateText[s] ?? s
}

function fileStateSeverity(s: string) {
  switch (s) {
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    case 'skipped':
      return 'warn'
    default:
      return 'info'
  }
}

async function onCreated() {
  await tasks.refresh()
  toast.add({ severity: 'success', summary: '任务已创建', life: 3000 })
}

const statusText: Record<string, string> = {
  queued: '排队中',
  running: '下载中',
  paused: '已暂停',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
}

function statusLabel(s: string) {
  return statusText[s] ?? s
}

function statusSeverity(s: string) {
  switch (s) {
    case 'running':
      return 'info'
    case 'done':
      return 'success'
    case 'failed':
      return 'danger'
    case 'paused':
      return 'warn'
    case 'canceled':
      return 'secondary'
    default:
      return 'secondary'
  }
}

function isFinal(s: string) {
  return s === 'done' || s === 'failed' || s === 'canceled' || s === 'paused'
}
</script>

<style scoped>
.mt-8 {
  margin-top: 8px;
}

.filter-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
}

.filter-toolbar .filter-search {
  flex: 1;
  min-width: 240px;
}

/* SearchBox 默认 inline-flex 宽度由内容决定，这里强制撑满并级联到内部输入框 */
.filter-toolbar .filter-search,
.filter-toolbar .filter-search :deep(.p-inputtext) {
  width: 100%;
}

.match-count {
  color: var(--p-text-muted-color);
  font-size: 12px;
  white-space: nowrap;
  margin-left: auto;
}

.regex-error {
  color: var(--p-red-500);
  font-size: 12px;
  padding: 4px 0;
}

.file-filter {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 0 8px;
}

.file-filter .grow {
  flex: 1;
}

/* 与任务级工具栏同理：强制 SearchBox 及其内部输入框撑满剩余宽度 */
.file-filter .grow,
.file-filter .grow :deep(.p-inputtext) {
  width: 100%;
}

.header-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
}

.total-speed {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 6px;
  background: var(--p-primary-50);
  color: var(--p-primary-700);
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
}

.total-speed .pi {
  font-size: 12px;
}

.meta {
  color: var(--p-text-muted-color);
  font-size: 13px;
  white-space: nowrap;
}

.task-meta {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  color: var(--p-text-muted-color);
  font-size: 12px;
}

.task-meta .pi {
  font-size: 12px;
  margin-right: 4px;
}

.file-row .size {
  color: var(--p-text-muted-color);
  font-size: 12px;
  white-space: nowrap;
}

.file-row .speed {
  color: var(--p-primary-500);
  font-size: 12px;
  white-space: nowrap;
  min-width: 72px;
  text-align: right;
}

.file-row .failed {
  color: var(--p-red-500);
}

.files-panel {
  border-top: 1px solid var(--p-content-border-color);
  padding-top: 8px;
}

.files-hint {
  color: var(--p-text-muted-color);
  font-size: 13px;
  padding: 4px 0;
}

.task-file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 0;
  font-size: 13px;
}

.task-file-row .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-file-row .size {
  color: var(--p-text-muted-color);
  font-size: 12px;
  white-space: nowrap;
}

.task-file-row .state-tag {
  font-size: 11px;
}

.files-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.file-actions {
  display: flex;
  gap: 2px;
  margin-left: auto;
}

.file-actions :deep(.p-button) {
  width: 28px;
  height: 28px;
}

.file-actions :deep(.p-button .p-button-icon) {
  font-size: 12px;
}

/* 顶部全局统计汇总条 */
.tasks-summary-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 6px;
  font-size: 12px;
  flex-wrap: wrap;
}

.sum-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--p-text-muted-color);
}

.sum-item.sum-success {
  color: var(--p-green-600);
  font-weight: 500;
}

.app-dark .sum-item.sum-success {
  color: var(--p-green-400);
}

.sum-item.sum-danger {
  color: var(--p-red-500);
  font-weight: 500;
}

.app-dark .sum-item.sum-danger {
  color: var(--p-red-400);
}

.sum-item.sum-warn {
  color: var(--p-orange-500);
  font-weight: 500;
}

.app-dark .sum-item.sum-warn {
  color: var(--p-orange-400);
}

/* 任务卡片头部状态统计徽章 */
.task-stats-badges {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  font-size: 12px;
  white-space: nowrap;
}

.stat-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 2px 7px;
  border-radius: 4px;
  font-weight: 500;
}

.stat-badge.stat-success {
  background: var(--p-green-50);
  color: var(--p-green-700);
}

.app-dark .stat-badge.stat-success {
  background: var(--p-green-950);
  color: var(--p-green-300);
}

.stat-badge.stat-danger {
  background: var(--p-red-50);
  color: var(--p-red-700);
}

.app-dark .stat-badge.stat-danger {
  background: var(--p-red-950);
  color: var(--p-red-300);
}

.stat-badge.stat-warn {
  background: var(--p-orange-50);
  color: var(--p-orange-700);
}

.app-dark .stat-badge.stat-warn {
  background: var(--p-orange-950);
  color: var(--p-orange-300);
}

.stat-badge.stat-total {
  color: var(--p-text-muted-color);
  padding: 0 2px;
}

.meta-stat-success {
  color: var(--p-green-600);
  font-weight: 500;
}

.app-dark .meta-stat-success {
  color: var(--p-green-400);
}

.meta-stat-failed {
  color: var(--p-red-500);
  font-weight: 500;
}

.app-dark .meta-stat-failed {
  color: var(--p-red-400);
}

.meta-stat-skipped {
  color: var(--p-orange-500);
  font-weight: 500;
}

.app-dark .meta-stat-skipped {
  color: var(--p-orange-400);
}

/* 文件过滤工具栏 */
.file-filter-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 4px 0 8px;
  flex-wrap: wrap;
}

.file-status-filter {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.file-search-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 220px;
  justify-content: flex-end;
}

.file-search-bar .grow,
.file-search-bar .grow :deep(.p-inputtext) {
  width: 100%;
}

.file-reason {
  color: var(--p-orange-600);
  background: var(--p-orange-50);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  white-space: nowrap;
}

.app-dark .file-reason {
  color: var(--p-orange-300);
  background: var(--p-orange-950);
}
</style>
