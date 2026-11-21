// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import AccessPage from './pages/AccessPage.vue'
import { authClient } from './api'

vi.mock('./api', () => ({
  authClient: {
    listUsers: vi.fn(),
    listRoles: vi.fn(),
    listAssignments: vi.fn(),
    listSessions: vi.fn(),
    listAccessLog: vi.fn(),
  },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: true,
    media: '',
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  }),
})

function user(username: string, displayName: string) {
  return { username, displayName, owner: false, disabled: false, locked: false, roles: [], lastLoginAt: '', state: 'active' }
}

function assignment(id: string, role: string) {
  return { id, role, scope: {}, activeFrom: '', expiresAt: '' }
}

async function flush() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('AccessPage selection race', () => {
  it('drops a slow assignments response for a previously selected user', async () => {
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [user('amy', 'Amy'), user('bob', 'Bob')] } as never)
    vi.mocked(authClient.listRoles).mockResolvedValue({ roles: [] } as never)
    vi.mocked(authClient.listSessions).mockResolvedValue({ sessions: [] } as never)
    let resolveAmy!: (value: never) => void
    vi.mocked(authClient.listAssignments).mockImplementation(((args: { username: string }) => {
      if (args.username === 'amy') return new Promise((resolve) => { resolveAmy = resolve as (value: never) => void })
      return Promise.resolve({ assignments: [assignment('b1', 'RoleB')] } as never)
    }) as never)

    const el = document.createElement('div')
    document.body.appendChild(el)
    const app = createApp({
      setup: () => () => h(AccessPage, { permissions: ['access:users'], username: 'root', recordTypes: [] }),
    })
    app.mount(el)
    cleanups.push(() => app.unmount())
    await flush()

    const rows = [...document.querySelectorAll('tbody .row-open')]
    expect(rows).toHaveLength(2)
    // Open Amy's sheet, then Bob's before Amy's assignments arrive.
    ;(rows[0] as HTMLElement).click()
    await flush()
    ;(rows[1] as HTMLElement).click()
    await flush()
    // Amy's slow response lands last and must lose.
    resolveAmy({ assignments: [assignment('a1', 'RoleA')] } as never)
    await flush()

    const sheet = document.querySelector('.access-assignments')?.textContent ?? ''
    expect(sheet).toContain('RoleB')
    expect(sheet).not.toContain('RoleA')
  })
})
