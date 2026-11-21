// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { Code, ConnectError } from '@connectrpc/connect'
import { setCsrfToken, shouldHandleSessionEnd } from './api'

afterEach(() => {
  setCsrfToken('')
})

function req(service: string, method: string) {
  return { service: { typeName: service }, method: { name: method } }
}

describe('shouldHandleSessionEnd', () => {
  it('ends the session for a signed-in desk on other procedures', () => {
    setCsrfToken('token')
    expect(shouldHandleSessionEnd(req('athena.v1.RecordService', 'DeleteRecord'), new ConnectError('e', Code.Unauthenticated))).toBe(true)
  })

  it('never signs out for the authentication calls themselves', () => {
    setCsrfToken('token')
    const err = new ConnectError('e', Code.Unauthenticated)
    expect(shouldHandleSessionEnd(req('athena.v1.AuthService', 'Login'), err)).toBe(false)
    expect(shouldHandleSessionEnd(req('athena.v1.AuthService', 'StepUp'), err)).toBe(false)
    expect(shouldHandleSessionEnd(req('athena.v1.AuthService', 'CompleteChallenge'), err)).toBe(false)
    // Other AuthService calls still end it.
    expect(shouldHandleSessionEnd(req('athena.v1.AuthService', 'RevokeSession'), err)).toBe(true)
  })

  it('ignores other codes and signed-out desks', () => {
    setCsrfToken('token')
    expect(shouldHandleSessionEnd(req('athena.v1.RecordService', 'DeleteRecord'), new ConnectError('e', Code.PermissionDenied))).toBe(false)
    setCsrfToken('')
    expect(shouldHandleSessionEnd(req('athena.v1.RecordService', 'DeleteRecord'), new ConnectError('e', Code.Unauthenticated))).toBe(false)
  })
})
