// The dirty guard for drawers and dialogs (§13.2): dismissing a dirty form
// asks first instead of closing. Pure so the matrix is unit-testable.
export type GuardDecision = { confirming: boolean; close: boolean }

export function guardNext(dirty: boolean, event: 'requestDismiss' | 'discard' | 'keep'): GuardDecision {
  if (event === 'discard') return { confirming: false, close: true }
  if (event === 'keep') return { confirming: false, close: false }
  if (dirty) return { confirming: true, close: false }
  return { confirming: false, close: true }
}
