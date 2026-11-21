import { describe, expect, it } from 'vitest'
import { groupNotifications, needsAction, notificationTarget } from './notifications'

describe('notifications', () => {
  it('routes records to the record and system kinds to their pages', () => {
    expect(notificationTarget({ kind: 'save_stopped', subjectType: 'note', subjectId: 'abc' })).toEqual({
      kind: 'record',
      recordType: 'note',
      recordId: 'abc',
    })
    expect(notificationTarget({ kind: 'period_closed', subjectType: 'period', subjectId: 'acme/main/p1' })).toEqual({ kind: 'page', page: 'ledger' })
    expect(notificationTarget({ kind: 'invite_accepted', subjectType: 'user', subjectId: 'u1' })).toEqual({ kind: 'page', page: 'access' })
    expect(notificationTarget({ kind: 'session_ended', subjectType: 'session', subjectId: 's1' })).toEqual({ kind: 'page', page: 'access' })
  })

  it('decides by kind so a Record Type named user cannot steal a link', () => {
    expect(notificationTarget({ kind: 'save_stopped', subjectType: 'user', subjectId: 'abc' }).kind).toBe('record')
    expect(notificationTarget({ kind: 'access_changed', subjectType: 'user', subjectId: 'u1' })).toEqual({ kind: 'page', page: 'access' })
  })

  it('interrupts only for kinds that need action', () => {
    expect(needsAction('save_stopped')).toBe(true)
    expect(needsAction('account_locked')).toBe(true)
    expect(needsAction('sign_in_failed')).toBe(true)
    expect(needsAction('invite_accepted')).toBe(false)
    expect(needsAction('access_changed')).toBe(false)
    expect(needsAction('session_ended')).toBe(false)
    expect(needsAction('period_closed')).toBe(false)
  })

  it('groups Today and Earlier on the local day', () => {
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 12, 0, 0).toISOString()
    const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 12, 0, 0).toISOString()
    const groups = groupNotifications([{ createdAt: today }, { createdAt: yesterday }, { createdAt: '' }])
    expect(groups.today).toHaveLength(1)
    expect(groups.earlier).toHaveLength(2)
  })
})
