import { describe, expect, it } from 'vitest'
import { edgeSide, isIOSSafari, swipeAllowed, swipeVerdict, SWIPE_COMMIT_PX, SWIPE_EDGE_PX } from './swipeNav'

describe('edgeSide', () => {
  it('takes 24 px from each edge', () => {
    expect(edgeSide(0, 375)).toBe('left')
    expect(edgeSide(SWIPE_EDGE_PX, 375)).toBe('left')
    expect(edgeSide(SWIPE_EDGE_PX + 1, 375)).toBeNull()
    expect(edgeSide(375, 375)).toBe('right')
    expect(edgeSide(375 - SWIPE_EDGE_PX, 375)).toBe('right')
    expect(edgeSide(375 - SWIPE_EDGE_PX - 1, 375)).toBeNull()
  })
})

describe('swipeVerdict', () => {
  it('commits past 80 px in the edge direction', () => {
    expect(swipeVerdict('left', SWIPE_COMMIT_PX, 0, 500)).toBe('go')
    expect(swipeVerdict('right', -SWIPE_COMMIT_PX, 0, 500)).toBe('go')
    expect(swipeVerdict('left', SWIPE_COMMIT_PX - 1, 0, 500)).toBe('cancel')
  })

  it('rejects the wrong direction and vertical drags', () => {
    expect(swipeVerdict('left', -100, 0, 200)).toBe('cancel')
    expect(swipeVerdict('right', 100, 0, 200)).toBe('cancel')
    expect(swipeVerdict('left', 100, 120, 200)).toBe('cancel')
  })

  it('commits a fast flick below the distance', () => {
    expect(swipeVerdict('left', 60, 10, 100)).toBe('go')
    expect(swipeVerdict('left', 60, 10, 400)).toBe('cancel')
  })
})

describe('swipeAllowed', () => {
  const open = { drawerOpen: false, inScroller: false, inField: false, iosSafariBack: false }

  it('passes in the clear', () => {
    expect(swipeAllowed('left', open)).toBe(true)
    expect(swipeAllowed('right', open)).toBe(true)
  })

  it('suppresses in drawers, scrollers and fields', () => {
    expect(swipeAllowed('left', { ...open, drawerOpen: true })).toBe(false)
    expect(swipeAllowed('left', { ...open, inScroller: true })).toBe(false)
    expect(swipeAllowed('right', { ...open, inField: true })).toBe(false)
  })

  it('leaves the iOS back edge to Safari but keeps forward', () => {
    const ios = { ...open, iosSafariBack: true }
    expect(swipeAllowed('left', ios)).toBe(false)
    expect(swipeAllowed('right', ios)).toBe(true)
  })
})

describe('isIOSSafari', () => {
  const safari = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1'
  const chrome = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0 Mobile/15E148 Safari/604.1'
  const desktop = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36'

  it('matches only iOS Safari', () => {
    expect(isIOSSafari(safari)).toBe(true)
    expect(isIOSSafari(chrome)).toBe(false)
    expect(isIOSSafari(desktop)).toBe(false)
  })
})
