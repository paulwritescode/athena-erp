import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { describe, expect, it } from 'vitest'
import { classify, describeFailure, isStepUpRequired, sessionEndReason } from './apiErrors'
import { AuthFailureSchema } from './gen/athena/v1/auth_pb'
import { signInProblems } from './signInRules'

function withFailure(code: Code, message: string, reason: string, unlocksAt = ''): ConnectError {
  const detail = create(AuthFailureSchema, { reason, unlocksAt })
  return new ConnectError(message, code, undefined, [{ desc: AuthFailureSchema, value: detail }])
}

describe('classify', () => {
  it('treats Unauthenticated as wrong credentials on sign-in and an ended session elsewhere', () => {
    const err = new ConnectError('wrong username or password, or the user is locked', Code.Unauthenticated)
    expect(classify(err, { signIn: true }).kind).toBe('wrong-credentials')
    expect(classify(err, { signIn: true }).message).toBe('Wrong username or password.')
    expect(classify(err).kind).toBe('session-expired')
  })

  it('reads locked and disabled from the detail, with the unlock time and command', () => {
    const locked = classify(withFailure(Code.FailedPrecondition, 'locked', 'locked', '2026-11-20T09:15:00Z'), { signIn: true, username: 'dave' })
    expect(locked.kind).toBe('locked')
    expect(locked.persistent).toBe(true)
    expect(locked.unlocksAt?.toISOString()).toBe('2026-11-20T09:15:00.000Z')
    expect(locked.message).toContain('athena user set dave --unlock')
    expect(locked.message).toContain('unlocks at')
    expect(classify(withFailure(Code.FailedPrecondition, 'locked', 'locked')).message).toContain('athena user set <name> --unlock')
    expect(classify(withFailure(Code.FailedPrecondition, 'disabled', 'disabled')).kind).toBe('disabled')
  })

  it('names the ended-session reason in plain words', () => {
    expect(classify(withFailure(Code.Unauthenticated, 'e', 'idle')).message).toContain('away too long')
    expect(classify(withFailure(Code.Unauthenticated, 'e', 'absolute')).message).toContain('time limit')
    expect(classify(withFailure(Code.Unauthenticated, 'e', 'revoked')).message).toContain('was ended')
    expect(classify(new ConnectError('e', Code.Unauthenticated)).message).toContain('ended')
  })

  it('reads a step-up cancel as Not changed', () => {
    const cancelled = classify(new ConnectError('step-up cancelled', Code.Canceled))
    expect(cancelled.kind).toBe('cancelled')
    expect(cancelled.message).toBe('Not changed.')
  })

  it('maps step-up and rate limits to their own kinds', () => {
    const stepUp = classify(withFailure(Code.FailedPrecondition, 'e', 'step_up_required'))
    expect(stepUp.kind).toBe('step-up')
    expect(stepUp.message).toContain('password')
    expect(classify(withFailure(Code.ResourceExhausted, 'e', 'rate_limited')).kind).toBe('rate-limited')
  })

  it('detects step-up and session reasons', () => {
    expect(isStepUpRequired(withFailure(Code.FailedPrecondition, 'e', 'step_up_required'))).toBe(true)
    expect(isStepUpRequired(new ConnectError('e', Code.FailedPrecondition))).toBe(false)
    expect(sessionEndReason(withFailure(Code.Unauthenticated, 'e', 'idle'))).toBe('idle')
    expect(sessionEndReason(new ConnectError('e', Code.Unauthenticated))).toBeNull()
    expect(sessionEndReason(new ConnectError('e', Code.PermissionDenied))).toBeNull()
  })

  it('does not call a plain FailedPrecondition a lock', () => {
    const failure = classify(new ConnectError('a cancelled document cannot be changed', Code.FailedPrecondition))
    expect(failure.kind).toBe('rejected')
    expect(failure.message).toBe('a cancelled document cannot be changed')
  })

  it('separates a CSRF mismatch from a missing permission', () => {
    expect(classify(withFailure(Code.PermissionDenied, 'csrf', 'csrf')).kind).toBe('csrf')
    const denied = classify(new ConnectError('no', Code.PermissionDenied), { action: 'save the Rule' })
    expect(denied.kind).toBe('denied')
    expect(denied.message).toBe('You do not have permission to save the Rule.')
  })

  it('separates a database outage from a server that cannot be reached', () => {
    const database = classify(new ConnectError('the database is not reachable', Code.Unavailable))
    expect(database.kind).toBe('database')
    expect(database.persistent).toBe(true)
    expect(classify(new ConnectError('Failed to fetch', Code.Unknown, undefined, undefined, new TypeError('Failed to fetch'))).kind).toBe('unreachable')
    expect(classify(new ConnectError('Bad Gateway', Code.Unavailable)).kind).toBe('unreachable')
  })

  it('shows an unexpected error with its request ID and nothing from the server text', () => {
    const err = new ConnectError('could not save: Error 1064 near SELECT', Code.Internal, { 'x-request-id': '01a1f5fd-aaaa' })
    const failure = classify(err)
    expect(failure.kind).toBe('unexpected')
    expect(failure.requestId).toBe('01a1f5fd-aaaa')
    expect(failure.message).not.toContain('SELECT')
    expect(describeFailure(failure)).toContain('Reference 01a1f5fd-aaaa')
    expect(classify(new TypeError('boom')).kind).toBe('unexpected')
  })

  it('maps a conflict and the plain rejections', () => {
    expect(classify(new ConnectError('stale', Code.Aborted)).kind).toBe('conflict')
    expect(classify(new ConnectError('Quantity must be above zero', Code.InvalidArgument)).message).toBe('Quantity must be above zero')
  })
})

describe('signInProblems', () => {
  it('flags empty fields and a short password', () => {
    expect(signInProblems('', '')).toEqual({ username: 'Enter your username.', password: 'Enter your password.' })
    expect(signInProblems('dave', 'short').password).toContain('12')
    expect(signInProblems(' ', 'a long test passphrase').username).toBeDefined()
    expect(signInProblems('dave', 'a long test passphrase')).toEqual({})
  })
})
