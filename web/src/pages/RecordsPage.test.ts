// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { Code, ConnectError } from '@connectrpc/connect'
import { ledgerAmount, postingBlockedMessage, postingOutcomeLabel, postingPreviewBlocked } from './RecordsPage.vue'

// The books-entry words (G6.10) read through the shared helpers, without
// mounting the page.
describe('posting outcome labels', () => {
  it('names every server outcome in plain words', () => {
    expect(postingOutcomeLabel('posted')).toBe('Recorded')
    expect(postingOutcomeLabel('replayed')).toBe('Already recorded')
    expect(postingOutcomeLabel('conflict')).toBe('Conflict')
    expect(postingOutcomeLabel('rejected')).toBe('Blocked')
    expect(postingOutcomeLabel('error')).toBe('Failed')
  })

  it('capitalizes an unknown outcome instead of showing it raw', () => {
    expect(postingOutcomeLabel('queued')).toBe('Queued')
    expect(postingOutcomeLabel('')).toBe('Attempt')
  })
})

describe('ledger amounts', () => {
  it('shows exact amounts and a dash for zero or empty', () => {
    expect(ledgerAmount('100.50')).toBe('100.50')
    expect(ledgerAmount('0.000001')).toBe('0.000001')
    expect(ledgerAmount('')).toBe('')
    expect(ledgerAmount('  ')).toBe('')
    expect(ledgerAmount('0')).toBe('')
    expect(ledgerAmount('0.00')).toBe('')
    expect(ledgerAmount('-0.0')).toBe('')
  })
})

describe('blocked-post messages', () => {
  it('maps a refused plan to a plain sentence with the reason', () => {
    const err = new ConnectError('The Rule bill-post cannot post: account 1000 is a group', Code.FailedPrecondition)
    expect(postingBlockedMessage(err, 'submit')).toBe('Not submitted: the books entry was blocked. Account 1000 is a group.')
    expect(postingBlockedMessage(err, 'cancel')).toBe('Not cancelled: the reversal was blocked. Account 1000 is a group.')
  })

  it('asks for a review when the plan changed since the preview', () => {
    const err = new ConnectError('the posting plan changed since it was previewed; review the voucher and submit again', Code.FailedPrecondition)
    expect(postingBlockedMessage(err, 'submit')).toBe('The books entry changed since you previewed it. Review the new preview and submit again.')
  })

  it('leaves other failures to their own error path', () => {
    expect(postingBlockedMessage(new Error('boom'), 'submit')).toBeNull()
    expect(postingBlockedMessage(new ConnectError('only a draft can be deleted', Code.FailedPrecondition), 'submit')).toBeNull()
    expect(postingBlockedMessage(new ConnectError('someone else posted this voucher just now; reload it and try again', Code.Aborted), 'submit')).toBeNull()
  })
})

describe('blocked-preview messages', () => {
  it('maps a refused plan to a plain sentence', () => {
    const err = new ConnectError('The Rule bill-post cannot post: period 2026-11 is closed', Code.FailedPrecondition)
    expect(postingPreviewBlocked(err)).toBe('The books entry is blocked. Period 2026-11 is closed.')
  })

  it('returns null for other failures', () => {
    expect(postingPreviewBlocked(new Error('boom'))).toBeNull()
    expect(postingPreviewBlocked(new ConnectError('invoice is not a submittable document', Code.FailedPrecondition))).toBeNull()
  })
})
