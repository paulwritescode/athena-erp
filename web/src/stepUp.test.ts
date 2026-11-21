// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { cancelStepUp, requestStepUpPassword, stepUpBusy, stepUpError, stepUpOpen, submitStepUp } from './stepUp'
import { authClient } from './api'
import { AuthFailureSchema } from './gen/athena/v1/auth_pb'

vi.mock('./api', () => ({
  authClient: { stepUp: vi.fn() },
  setCsrfToken: vi.fn(),
  setStepUpAsker: vi.fn(),
  emitSessionEnded: vi.fn(),
}))

afterEach(() => {
  cancelStepUp()
  vi.restoreAllMocks()
})

describe('stepUp', () => {
  it('opens once for concurrent callers and resolves all on submit', async () => {
    vi.mocked(authClient.stepUp).mockResolvedValue({ session: { csrfToken: 'new' } } as never)
    const first = requestStepUpPassword()
    const second = requestStepUpPassword()
    expect(stepUpOpen.value).toBe(true)
    await submitStepUp('pw')
    expect(stepUpOpen.value).toBe(false)
    await expect(first).resolves.toBe('pw')
    await expect(second).resolves.toBe('pw')
    expect(authClient.stepUp).toHaveBeenCalledTimes(1)
  })

  it('cancelling resolves null and closes', async () => {
    const waiting = requestStepUpPassword()
    cancelStepUp()
    await expect(waiting).resolves.toBeNull()
    expect(stepUpOpen.value).toBe(false)
    expect(authClient.stepUp).not.toHaveBeenCalled()
  })

  it('shows a wrong password inline and stays open', async () => {
    vi.mocked(authClient.stepUp).mockRejectedValueOnce(new ConnectError('nope', Code.Unauthenticated))
    const waiting = requestStepUpPassword()
    await submitStepUp('wrong')
    expect(stepUpOpen.value).toBe(true)
    expect(stepUpBusy.value).toBe(false)
    expect(stepUpError.value).toBe('That password is not right.')
    vi.mocked(authClient.stepUp).mockResolvedValue({ session: { csrfToken: 'new' } } as never)
    await submitStepUp('right')
    await expect(waiting).resolves.toBe('right')
    await nextTick()
  })

  it('ends the session when it died mid-dialog instead of asking again', async () => {
    const { emitSessionEnded } = await import('./api')
    const detail = create(AuthFailureSchema, { reason: 'revoked' })
    vi.mocked(authClient.stepUp).mockRejectedValueOnce(
      new ConnectError('gone', Code.Unauthenticated, undefined, [{ desc: AuthFailureSchema, value: detail }]),
    )
    const waiting = requestStepUpPassword()
    await submitStepUp('right')
    expect(emitSessionEnded).toHaveBeenCalledWith('revoked')
    expect(stepUpOpen.value).toBe(false)
    await expect(waiting).resolves.toBeNull()
  })

  it('asks for a password when empty', async () => {
    requestStepUpPassword()
    await submitStepUp('')
    expect(stepUpError.value).toContain('password')
    expect(authClient.stepUp).not.toHaveBeenCalled()
  })
})
