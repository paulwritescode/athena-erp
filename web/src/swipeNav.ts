// Edge-swipe back and forward (§13.3), kept pure so thresholds and
// suppression are unit-testable. The composable gathers the DOM context.
export const SWIPE_EDGE_PX = 24
export const SWIPE_COMMIT_PX = 80
export const SWIPE_FLICK_MS = 300
export const SWIPE_FLICK_PX_PER_MS = 0.5
export const SWIPE_SETTLE_MS = 240

export type SwipeSide = 'left' | 'right'

export function edgeSide(x: number, viewportWidth: number): SwipeSide | null {
  if (x <= SWIPE_EDGE_PX) return 'left'
  if (x >= viewportWidth - SWIPE_EDGE_PX) return 'right'
  return null
}

export function swipeVerdict(side: SwipeSide, dx: number, dy: number, dtMs: number): 'go' | 'cancel' {
  // Left-edge swipes travel right, right-edge swipes travel left.
  const forward = side === 'left' ? dx : -dx
  if (forward <= 0) return 'cancel'
  if (Math.abs(dy) > Math.abs(dx)) return 'cancel'
  if (forward >= SWIPE_COMMIT_PX) return 'go'
  if (dtMs <= SWIPE_FLICK_MS && forward / Math.max(dtMs, 1) >= SWIPE_FLICK_PX_PER_MS) return 'go'
  return 'cancel'
}

export type SwipeContext = {
  drawerOpen: boolean
  inScroller: boolean
  inField: boolean
  iosSafariBack: boolean
}

export function swipeAllowed(side: SwipeSide, context: SwipeContext): boolean {
  if (context.drawerOpen || context.inScroller || context.inField) return false
  // iOS Safari already swipes back from the left edge; never add a second one.
  if (side === 'left' && context.iosSafariBack) return false
  return true
}

// iOS Safari: WebKit without another browser's token. Pure over the UA so it
// is testable; touch is implied by a touchstart having happened.
export function isIOSSafari(userAgent: string): boolean {
  return /iPad|iPhone|iPod/.test(userAgent) && /WebKit/.test(userAgent) && !/(CriOS|FxiOS|EdgiOS|Chrome|Android)/.test(userAgent)
}
