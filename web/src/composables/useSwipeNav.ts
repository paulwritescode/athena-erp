// useSwipeNav wires the §13.3 edge swipe: a horizontal swipe from the screen
// edge goes back (left) or forward (right) through history. The page follows
// the finger and settles over 240 ms; reduced motion navigates instantly.
import { onMounted, onUnmounted } from 'vue'
import { edgeSide, isIOSSafari, swipeAllowed, swipeVerdict, SWIPE_SETTLE_MS, type SwipeSide } from '../swipeNav'

const DRAWER_CLASSES = ['has-sheet', 'has-sheet-wide', 'has-drawer']

function inHorizontalScroller(target: EventTarget | null): boolean {
  let node = target as HTMLElement | null
  while (node && node !== document.body) {
    if (node instanceof HTMLElement) {
      const style = getComputedStyle(node)
      const scrollable = style.overflowX === 'auto' || style.overflowX === 'scroll'
      if (scrollable && node.scrollWidth > node.clientWidth + 1) return true
    }
    node = node.parentElement
  }
  return false
}

function inField(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.closest('input, textarea, select, [contenteditable="true"]') !== null
}

export function useSwipeNav(): void {
  let tracking: { side: SwipeSide; startX: number; startY: number; startT: number; reduced: boolean } | null = null

  function page(): HTMLElement | null {
    return document.querySelector<HTMLElement>('[data-page-main]')
  }

  function setFollow(dx: number) {
    const el = page()
    if (el) el.style.transform = dx === 0 ? '' : `translateX(${dx}px)`
  }

  function settle() {
    const el = page()
    if (!el) return
    el.style.transition = `transform ${SWIPE_SETTLE_MS}ms cubic-bezier(0.23, 1, 0.32, 1)`
    el.style.transform = ''
    window.setTimeout(() => {
      el.style.transition = ''
    }, SWIPE_SETTLE_MS)
  }

  function onStart(event: TouchEvent) {
    if (event.touches.length !== 1) {
      tracking = null
      return
    }
    const touch = event.touches[0]
    if (!touch) return
    const side = edgeSide(touch.clientX, window.innerWidth)
    if (!side) return
    const drawerOpen = DRAWER_CLASSES.some((cls) => document.body.classList.contains(cls))
    const context = {
      drawerOpen,
      inScroller: inHorizontalScroller(event.target),
      inField: inField(event.target),
      iosSafariBack: isIOSSafari(navigator.userAgent),
    }
    if (!swipeAllowed(side, context)) return
    tracking = {
      side,
      startX: touch.clientX,
      startY: touch.clientY,
      startT: performance.now(),
      reduced: window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    }
  }

  function onMove(event: TouchEvent) {
    if (!tracking || event.touches.length !== 1) return
    const touch = event.touches[0]
    if (!touch || tracking.reduced) return
    const dx = touch.clientX - tracking.startX
    const forward = tracking.side === 'left' ? dx : -dx
    setFollow(forward > 0 ? dx : 0)
  }

  function onEnd(event: TouchEvent) {
    if (!tracking) return
    const current = tracking
    tracking = null
    const touch = event.changedTouches[0]
    if (!touch) {
      setFollow(0)
      return
    }
    const verdict = swipeVerdict(
      current.side,
      touch.clientX - current.startX,
      touch.clientY - current.startY,
      performance.now() - current.startT,
    )
    if (verdict === 'go') {
      setFollow(0)
      if (current.side === 'left') window.history.back()
      else window.history.forward()
    } else if (current.reduced) {
      setFollow(0)
    } else {
      settle()
    }
  }

  function onCancel() {
    tracking = null
    setFollow(0)
  }

  onMounted(() => {
    window.addEventListener('touchstart', onStart, { passive: true })
    window.addEventListener('touchmove', onMove, { passive: true })
    window.addEventListener('touchend', onEnd, { passive: true })
    window.addEventListener('touchcancel', onCancel)
  })
  onUnmounted(() => {
    window.removeEventListener('touchstart', onStart)
    window.removeEventListener('touchmove', onMove)
    window.removeEventListener('touchend', onEnd)
    window.removeEventListener('touchcancel', onCancel)
  })
}
