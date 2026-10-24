import { defineStore } from 'pinia'
import { Chat } from '../api'
import type { DialogView } from '../types'

let generation = 0

/** 对话列表（私聊 / 群组 / 频道），进入对话页时按需加载。 */
export const useChatsStore = defineStore('chats', {
  state: () => ({
    dialogs: [] as DialogView[],
    loading: false,
    loaded: false,
    error: '',
  }),
  actions: {
    /** 首次进入页面时加载（未登录等场景静默跳过，由页面提示）。 */
    async ensureLoaded() {
      if (this.loaded || this.loading) return
      await this.refresh()
    },
    /** 重新拉取对话列表；错误记录到 error 并重新抛出，由页面 toast。 */
    async refresh() {
      const requestGeneration = generation
      this.loading = true
      this.error = ''
      try {
        const dialogs = (await Chat.listDialogs()) ?? []
        if (requestGeneration !== generation) return
        this.dialogs = dialogs
        this.loaded = true
      } catch (e: any) {
        if (requestGeneration !== generation) return
        this.error = String(e?.message ?? e)
        throw e
      } finally {
        if (requestGeneration === generation) this.loading = false
      }
    },
    /** 按 ID 查找对话（详情页标题兜底）。 */
    byId(id: number, typ?: string): DialogView | undefined {
      return this.dialogs.find((d) => d.id === id && (!typ || d.type === typ))
    },
    /** 登出后清空缓存。 */
    reset() {
      generation++
      this.loading = false
      this.dialogs = []
      this.loaded = false
      this.error = ''
    },
  },
})
