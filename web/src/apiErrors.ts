import { Code, ConnectError } from '@connectrpc/connect'
import { AuthFailureSchema } from './gen/athena/v1/auth_pb'

// One place that turns what a ConnectRPC call can throw into a state the desk
// shows. The server's codes are: Unauthenticated (wrong credentials, or no
// session), PermissionDenied (a missing permission, or a CSRF token that does
// not match), FailedPrecondition with an AuthFailure detail (locked or disabled
// account, sent only for the right password), Unavailable (the database is not
// reachable) and Internal for the rest. A network failure never reaches the
// server and arrives with a TypeError cause.
export type FailureKind =
  | 'wrong-credentials'
  | 'locked'
  | 'disabled'
  | 'session-expired'
  | 'step-up'
  | 'rate-limited'
  | 'cancelled'
  | 'unreachable'
  | 'database'
  | 'csrf'
  | 'denied'
  | 'conflict'
  | 'rejected'
  | 'unexpected'

/** True when the server asks for the password again before a destructive action. */
export function isStepUpRequired(err: unknown): boolean {
  return (
    err instanceof ConnectError &&
    err.code === Code.FailedPrecondition &&
    err.findDetails(AuthFailureSchema)[0]?.reason === 'step_up_required'
  )
}

/** The ended-session reason the server attached, if any. */
export function sessionEndReason(err: unknown): string | null {
  if (!(err instanceof ConnectError) || err.code !== Code.Unauthenticated) return null
  const reason = err.findDetails(AuthFailureSchema)[0]?.reason ?? ''
  return reason === 'idle' || reason === 'absolute' || reason === 'revoked' ? reason : null
}

export type Failure = {
  kind: FailureKind
  /** Short text safe to show. Never a stack trace or SQL. */
  message: string
  /** A condition that stays until something changes, so it belongs in the page, not a toast. */
  persistent: boolean
  /** Correlation ID for support, when the server sent one. */
  requestId?: string
  /** When a lock ends, as a Date, if the server knows. */
  unlocksAt?: Date
}

export type Context = {
  /** The call is the sign-in itself, so Unauthenticated means wrong credentials. */
  signIn?: boolean
  /** What the person was doing, as in "save the Rule", for messages about permission. */
  action?: string
  /** Name of the account, to give the exact unlock command. */
  username?: string
}

export const requestIdHeader = 'x-request-id'

function clock(date: Date): string {
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/** Maps anything a call threw to a Failure. */
export function classify(err: unknown, context: Context = {}): Failure {
  if (!(err instanceof ConnectError)) {
    return { kind: 'unexpected', message: 'Something went wrong.', persistent: false }
  }
  const requestId = err.metadata.get(requestIdHeader) ?? undefined
  const base = { requestId }
  // connect-web reports a failed fetch (server down, offline) as Unknown with
  // the TypeError as its cause; it never reached the server.
  if (err.cause instanceof TypeError) return { ...base, kind: 'unreachable', message: 'Cannot reach the Athena server. Check that it is running, then try again.', persistent: true }
  switch (err.code) {
    case Code.Unauthenticated: {
      if (context.signIn) return { ...base, kind: 'wrong-credentials', message: 'Wrong username or password.', persistent: false }
      const reason = sessionEndReason(err)
      if (reason === 'idle') return { ...base, kind: 'session-expired', message: 'Your session ended because you were away too long. Sign in again.', persistent: false }
      if (reason === 'absolute') return { ...base, kind: 'session-expired', message: 'Your session reached its time limit. Sign in again.', persistent: false }
      if (reason === 'revoked') return { ...base, kind: 'session-expired', message: 'Your session was ended. Sign in again.', persistent: false }
      return { ...base, kind: 'session-expired', message: 'Your session ended. Sign in again.', persistent: false }
    }
    case Code.PermissionDenied: {
      const detail = err.findDetails(AuthFailureSchema)[0]
      if (detail?.reason === 'csrf') return { ...base, kind: 'csrf', message: 'This page is out of date. Reload it and try again.', persistent: false }
      return { ...base, kind: 'denied', message: `You do not have permission to ${context.action ?? 'do this'}.`, persistent: false }
    }
    case Code.FailedPrecondition: {
      const detail = err.findDetails(AuthFailureSchema)[0]
      if (detail?.reason === 'step_up_required') return { ...base, kind: 'step-up', message: 'Enter your password to continue.', persistent: false }
      if (detail?.reason === 'locked') {
        const unlocksAt = detail.unlocksAt ? new Date(detail.unlocksAt) : undefined
        const unlock = context.username ? `athena user set ${context.username} --unlock` : 'athena user set <name> --unlock'
        const when = unlocksAt && !Number.isNaN(unlocksAt.getTime()) ? `It unlocks at ${clock(unlocksAt)}. ` : ''
        return { ...base, kind: 'locked', message: `This account is locked after too many failed sign-ins. ${when}Ask an owner to unlock it, or run ${unlock} on the server.`, persistent: true, unlocksAt }
      }
      if (detail?.reason === 'disabled') return { ...base, kind: 'disabled', message: 'This account is disabled. Ask an owner to enable it.', persistent: true }
      return { ...base, kind: 'rejected', message: err.rawMessage, persistent: false }
    }
    case Code.Aborted:
      return { ...base, kind: 'conflict', message: 'Someone else changed this. Reload it, then apply your change again.', persistent: false }
    case Code.Unavailable:
      return !/database/i.test(err.rawMessage)
        ? { ...base, kind: 'unreachable', message: 'Cannot reach the Athena server. Check that it is running, then try again.', persistent: true }
        : { ...base, kind: 'database', message: 'The database is not connected. Start MariaDB and check ATHENA_DATABASE_URL, then try again.', persistent: true }
    case Code.ResourceExhausted: {
      const detail = err.findDetails(AuthFailureSchema)[0]
      const wait = Number(detail?.retryAfterSeconds ?? 0)
      return {
        ...base,
        kind: 'rate-limited',
        message: wait > 0 ? `Too many tries. Wait ${wait} second${wait === 1 ? '' : 's'} and try again.` : 'Too many tries. Wait a moment and try again.',
        persistent: false,
      }
    }
    case Code.Canceled:
      return { ...base, kind: 'cancelled', message: 'Not changed.', persistent: false }
    case Code.InvalidArgument:
    case Code.NotFound:
    case Code.AlreadyExists:
      return { ...base, kind: 'rejected', message: err.rawMessage, persistent: false }
    default:
      return { ...base, kind: 'unexpected', message: 'Something went wrong on the server.', persistent: false }
  }
}

/** The message with its request ID, for toasts about failures nobody can fix at the desk. */
export function describeFailure(failure: Failure): string {
  return failure.requestId ? `${failure.message} Reference ${failure.requestId}.` : failure.message
}
