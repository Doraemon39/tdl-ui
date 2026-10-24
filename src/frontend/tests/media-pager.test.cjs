const test = require('node:test')
const assert = require('node:assert/strict')
const { loadTs } = require('./load-ts.cjs')

const item = id => ({ dialogId: 7, messageId: id, name: `v${id}.mp4`, size: 3, mediaId: `document:${id}`, date: 1700000000 })
function fixture(listMedia) {
  const state = { owner: 42, typ: 'channel', ready: true }
  const { useMediaPager } = loadTs('composables/useMediaPager.ts', { '../api': { Chat: { listMedia } } })
  const pager = useMediaPager({ accountId: () => state.owner, dialogType: () => state.typ, ready: () => state.ready })
  pager.switchTo(7)
  return { pager, state }
}

test('full scan joins every page and deduplicates messages', async () => {
  const { pager } = fixture(async q => q.offsetId === 0 ? { items: [item(3), item(2)], nextOffset: 2 } : { items: [item(2), item(1)], nextOffset: 0 })
  const result = await pager.loadAll()
  assert.equal(result.status, 'complete')
  assert.deepEqual(result.items.map(i => i.messageId), [3, 2, 1])
})

test('cancellation cannot return partial results as complete', async () => {
  let release
  const { pager } = fixture(() => new Promise(resolve => { release = resolve }))
  const pending = pager.loadAll()
  pager.cancelScan()
  release({ items: [item(3)], nextOffset: 0 })
  const result = await pending
  assert.equal(result.status, 'cancelled')
  assert.deepEqual(result.items, [])
})

test('unchanged cursor fails closed', async () => {
  const { pager } = fixture(async () => ({ items: [item(3)], nextOffset: 3 }))
  assert.equal((await pager.loadAll()).status, 'failed')
  assert.equal(pager.failed.value, true)
})

test('account switch discards in-flight media', async () => {
  let release
  const { pager, state } = fixture(() => new Promise(resolve => { release = resolve }))
  const pending = pager.loadAll()
  state.owner = 99
  release({ items: [item(3)], nextOffset: 0 })
  assert.equal((await pending).status, 'cancelled')
  assert.equal(pager.items.value.length, 0)
})

test('snapshots do not cross peer namespaces', async () => {
  const { pager, state } = fixture(async () => ({ items: [item(3)], nextOffset: 0 }))
  await pager.loadMore()
  pager.saveSnapshot()
  state.typ = 'private'
  assert.equal(pager.switchTo(7), false)
  assert.equal(pager.items.value.length, 0)
})

test('whole-dialog scan ignores a previous date jump and filter', async () => {
  const queries = []
  const { pager } = fixture(async q => { queries.push(q); return { items: [], nextOffset: 0 } })
  pager.applied.value.query = 'old filter'
  const result = await pager.loadAll(true)
  assert.equal(result.status, 'complete')
  assert.equal(queries[0].query, '')
  assert.equal(queries[0].offsetId, 0)
})
