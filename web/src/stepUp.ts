// Step-up (B7, R7.3): root and builders re-enter their password before a
// destructive action once 15 minutes pass since the last proof. The server
// answers step_up_required; the api interceptor below asks once (concurrent
// calls share one dialog), calls StepUp, rotates the CSRF token the response
// carries, and retries the original call once. Forms keep their state: the
// retry re-invokes the caller's closure, and cancelling rethrows the
// original error so the caller shows its usual message.
import { ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { authClient, emitSessionEnded, setCsrfToken, setStepUpAsker } from './api'
import { classify, sessionEndReason } from './apiErrors'

export const stepUpOpen = ref(false)
export const stepUpBusy = ref(false)
export const stepUpError = ref('')

type Waiter = { resolve: (password: string | null) => void }
let waiters: Waiter[] = []

/** Opens the password dialog (once for concurrent callers) and waits. */
export function requestStepUpPassword(): Promise<string | null> {
  return new Promise((resolve) => {
    waiters.push({ resolve })
    stepUpError.value = ''
    stepUpOpen.value = true
  })
}

export function cancelStepUp(): void {
  const pending = waiters
  waiters = []
  stepUpOpen.value = false
  stepUpBusy.value = false
  for (const waiter of pending) waiter.resolve(null)
}

export async function submitStepUp(password: string): Promise<boolean> {
  if (!password) {
    stepUpError.value = 'Enter your password.'
    return false
  }
  stepUpBusy.value = true
  stepUpError.value = ''
  try {
    const res = await authClient.stepUp({ password })
    if (res.session?.csrfToken) setCsrfToken(res.session.csrfToken)
  } catch (err) {
    // A bare Unauthenticated is a wrong password: say so inline and stay
    // open (the interceptor never signs out for StepUp itself). A reasoned
    // one means the session died mid-dialog, so end it with that reason.
    if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
      const reason = sessionEndReason(err)
      if (reason) {
        cancelStepUp()
        emitSessionEnded(reason)
        return false
      }
      stepUpError.value = 'That password is not right.'
      stepUpBusy.value = false
      return false
    }
    stepUpError.value = classify(err, { action: 'prove it is you' }).message
    stepUpBusy.value = false
    return false
  }
  const pending = waiters
  waiters = []
  stepUpOpen.value = false
  stepUpBusy.value = false
  for (const waiter of pending) waiter.resolve(password)
  return true
}

// Registers with the api interceptor (one import direction, no cycle).
setStepUpAsker(requestStepUpPassword)
