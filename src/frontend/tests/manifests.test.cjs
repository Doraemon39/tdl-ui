const test = require('node:test')
const assert = require('node:assert/strict')
const { loadTs } = require('./load-ts.cjs')
const { manifestScopeMatches, manifestEntryMatches } = loadTs('utils/manifests.ts')

test('manifest scope includes account and peer namespace', () => {
  const info = { header: { accountId: 42, dialogId: 7, dialogType: 'supergroup' } }
  assert.equal(manifestScopeMatches(info, 42, 7, 'channel'), true)
  assert.equal(manifestScopeMatches(info, 99, 7, 'channel'), false)
  assert.equal(manifestScopeMatches(info, 42, 7, 'private'), false)
  assert.equal(manifestScopeMatches(info, 42, 7, ''), false)
})

test('pending, edited or unconfirmed media never matches a manifest', () => {
  const item = { messageId: 1, mediaId: 'document:10', size: 3 }
  const entry = { ...item, state: 'done', evidence: 'source', date: 1700000000 }
  assert.equal(manifestEntryMatches(item, entry), true)
  assert.equal(manifestEntryMatches(item, { ...entry, state: 'pending' }), false)
  assert.equal(manifestEntryMatches(item, { ...entry, mediaId: 'document:11' }), false)
  assert.equal(manifestEntryMatches(item, { ...entry, evidence: '' }), false)
})
