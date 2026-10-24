import type { ManifestInfo, MediaItem } from '../types'
import { peerNamespace } from './format'

export interface ManifestSelection { info: ManifestInfo; mode?: string }

export function manifestScopeMatches(info: ManifestInfo, accountId: number, dialogId: number, dialogType: string): boolean {
  const h = info.header
  const typ = peerNamespace(dialogType)
  return accountId > 0 && dialogId > 0 && !!typ && h.accountId === accountId && h.dialogId === dialogId && peerNamespace(h.dialogType) === typ
}

export function manifestEntryMatches(item: MediaItem, entry: ManifestInfo['entries'][number] | undefined, _mode?: string): boolean {
  return !!entry && entry.state === 'done' && ['source', 'local_unique', 'manual'].includes(entry.evidence) && entry.date > 0 && item.messageId === entry.messageId && !!item.mediaId && item.mediaId === entry.mediaId && item.size === entry.size
}
