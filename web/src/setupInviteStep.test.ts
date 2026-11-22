// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SetupInviteStep from './components/setup/SetupInviteStep.vue'
import { authClient } from './api'

vi.mock('./api', () => ({
  authClient: { listUsers: vi.fn(), inviteUser: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function mountStep(canAccess: boolean, onDone: (value: unknown) => void) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SetupInviteStep, { canAccess, onDone }) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

function button(label: string): HTMLButtonElement {
  const found = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes(label))
  if (!found) throw new Error(`missing button ${label}`)
  return found as HTMLButtonElement
}

async function setInput(id: string, value: string) {
  const input = document.querySelector(`#${id}`) as HTMLInputElement | null
  if (!input) throw new Error(`missing #${id}`)
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

describe('SetupInviteStep', () => {
  it('invites an owner and shows the code once', async () => {
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [] } as never)
    vi.mocked(authClient.inviteUser).mockResolvedValue({ user: { username: 'alice' }, challenge: 'code-123' } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    await setInput('setup-owner-username', 'Alice')
    await setInput('setup-owner-name', 'Alice')
    button('Invite owner & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const invite = vi.mocked(authClient.inviteUser)
    expect(invite).toHaveBeenCalledTimes(1)
    expect(invite.mock.calls[0][0]).toEqual({ username: 'alice', displayName: 'Alice', roles: ['Instance Owner'] })
    expect(document.body.textContent).toContain('Invite code for alice')
    expect(document.body.textContent).toContain('code-123')
    button('Continue').click()
    await nextTick()
    expect(done).toEqual([{ owners: ['alice'], skipped: false }])
  })

  it('copies the invite code', async () => {
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [] } as never)
    vi.mocked(authClient.inviteUser).mockResolvedValue({ user: { username: 'alice' }, challenge: 'code-123' } as never)
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    await mountStep(true, () => {})
    await setInput('setup-owner-username', 'alice')
    button('Invite owner & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    button('Copy').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(writeText).toHaveBeenCalledWith('code-123')
  })

  it('rejects a bad username inline without inviting', async () => {
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [] } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    await setInput('setup-owner-username', 'a')
    button('Invite owner & continue').click()
    await nextTick()
    expect(document.body.textContent).toContain('2 to 63')
    expect(vi.mocked(authClient.inviteUser)).not.toHaveBeenCalled()
    expect(done).toEqual([])
  })

  it('continues when owners already exist, skipping root', async () => {
    vi.mocked(authClient.listUsers).mockResolvedValue({
      users: [
        { username: 'administrator', owner: true },
        { username: 'alice', owner: true },
        { username: 'bob', owner: false },
      ],
    } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    expect(document.body.textContent).toContain('Owners so far: alice.')
    button('Continue').click()
    await nextTick()
    expect(done).toEqual([{ owners: ['alice'], skipped: false }])
  })

  it('skips without an owner', async () => {
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [] } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    button('Skip for now').click()
    await nextTick()
    expect(vi.mocked(authClient.inviteUser)).not.toHaveBeenCalled()
    expect(done).toEqual([{ owners: [], skipped: true }])
  })
})
