import { describe, expect, it } from 'vitest'
import { guardNext } from './components/dismissGuard'

describe('guardNext', () => {
  it('closes a clean form at once', () => {
    expect(guardNext(false, 'requestDismiss')).toEqual({ confirming: false, close: true })
  })

  it('asks before closing a dirty form', () => {
    expect(guardNext(true, 'requestDismiss')).toEqual({ confirming: true, close: false })
  })

  it('discards or keeps from the confirm', () => {
    expect(guardNext(true, 'discard')).toEqual({ confirming: false, close: true })
    expect(guardNext(true, 'keep')).toEqual({ confirming: false, close: false })
  })
})
