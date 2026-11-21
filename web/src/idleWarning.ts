// The idle warning (B7, R7.7): two minutes before idle expiry the desk
// offers "Stay signed in" (Touch, the one active call that extends idle and
// reschedules from the fresh expiry) or "Sign out". Any activity here or in
// another tab dismisses it. Dismissing re-reads the truth with a passive
// GetSession: background polls never extend idle server-side, so the only
// drift comes from real user calls, and the re-read picks their new expiry
// up. If the truth is still inside two minutes the warning reopens, as it
// should: wiggling the mouse is not activity the server counts.
import { ref } from 'vue'
import { authClient } from './api'
import { postSessionMessage } from './sessionChannel'

export const WARN_BEFORE_MS = 2 * 60 * 1000
const RECHECK_MS = 60 * 1000
const ACTIVITY_BROADCAST_MS = 60 * 1000

export const idleWarnOpen = ref(false)
export const idleWarnBusy = ref(false)
export const idleWarnError = ref('')

let timer: ReturnType<typeof setTimeout> | undefined
let lastBroadcast = 0
let lastRecheck = 0
let tracking = false
let signOut: () => void = () => {}

export function setIdleSignOut(fn: () => void) {
  signOut = fn
}

export function rescheduleIdleWarning(idleExpiresAt: string): void {
  clearIdleWarning()
  if (!idleExpiresAt) return
  const expires = new Date(idleExpiresAt).getTime()
  if (Number.isNaN(expires)) return
  const delay = expires - WARN_BEFORE_MS - Date.now()
  if (delay <= 0) {
    idleWarnOpen.value = true
    return
  }
  timer = setTimeout(() => {
    timer = undefined
    idleWarnOpen.value = true
  }, delay)
}

export function clearIdleWarning(): void {
  if (timer !== undefined) clearTimeout(timer)
  timer = undefined
  idleWarnOpen.value = false
  idleWarnBusy.value = false
  idleWarnError.value = ''
}

/** A sign the session is alive (activity anywhere, a good poll): stand down. */
export function noteAlive(): void {
  if (!idleWarnOpen.value || idleWarnBusy.value) return
  idleWarnOpen.value = false
  void recheckAfterDismiss()
}

async function recheckAfterDismiss(): Promise<void> {
  const now = Date.now()
  if (now - lastRecheck < RECHECK_MS) return
  lastRecheck = now
  try {
    const res = await authClient.getSession({})
    if (res.session?.authenticated) rescheduleIdleWarning(res.session.idleExpiresAt)
  } catch {
    // A failed recheck leaves the warning stood down; a dead session
    // surfaces through the next real call instead.
  }
}

function onActivity() {
  noteAlive()
  const now = Date.now()
  if (now - lastBroadcast < ACTIVITY_BROADCAST_MS) return
  lastBroadcast = now
  postSessionMessage('activity')
}

export function startIdleTracking(): void {
  if (tracking) return
  tracking = true
  window.addEventListener('pointerdown', onActivity, { passive: true })
  window.addEventListener('keydown', onActivity)
}

export async function staySignedIn(): Promise<void> {
  if (idleWarnBusy.value) return
  idleWarnBusy.value = true
  idleWarnError.value = ''
  try {
    const res = await authClient.touch({})
    rescheduleIdleWarning(res.session?.idleExpiresAt ?? '')
    postSessionMessage('activity')
  } catch {
    idleWarnError.value = 'Could not reach the server. Try again, or sign out and back in.'
    idleWarnBusy.value = false
  }
}

export function giveUpSession(): void {
  clearIdleWarning()
  signOut()
}

/** Test-only reset for module state. */
export function resetIdleWarningForTests(): void {
  clearIdleWarning()
  lastBroadcast = 0
  lastRecheck = 0
  tracking = false
}
