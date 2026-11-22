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

async function flush() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

// Root administrator desk tests (ADR-0018, task B8): the root row offers no
// user action and its sheet says the account cannot change here. Slice 2
// restyles these refusals as disabled actions with reasons.
async function mountAccess() {
  vi.mocked(authClient.listUsers).mockResolvedValue({ users: [user('administrator', 'Administrator'), user('amy', 'Amy')] } as never)
  vi.mocked(authClient.listRoles).mockResolvedValue({ roles: [] } as never)
  vi.mocked(authClient.listAssignments).mockResolvedValue({ assignments: [] } as never)
  vi.mocked(authClient.listSessions).mockResolvedValue({ sessions: [] } as never)
  vi.mocked(authClient.listAccessLog).mockResolvedValue({ events: [] } as never)
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({
    setup: () => () => h(AccessPage, { permissions: ['access:users', 'access:roles', 'access:assign', 'audit'], username: 'administrator', recordTypes: [] }),
  })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await flush()
}

async function openRootSheet() {
  const rows = [...document.querySelectorAll('tbody tr')]
  expect(rows).toHaveLength(2)
  ;(rows[0] as HTMLElement).click()
  await flush()
  const sheet = document.querySelector('[role="dialog"]')
  expect(sheet).not.toBeNull()
  return sheet as HTMLElement
}

describe('AccessPage root account', () => {
  it('offers no suspend action on the root row', async () => {
    await mountAccess()
    // Only Amy's row gets the row-actions menu; the root row has none.
    const menus = [...document.querySelectorAll('button[aria-label^="Actions for row"]')]
    expect(menus).toHaveLength(1)
    const sheet = await openRootSheet()
    expect(sheet.textContent ?? '').not.toContain('Suspend')
  })

  it('says the root account cannot be changed here', async () => {
    await mountAccess()
    const sheet = await openRootSheet()
    expect(sheet.textContent ?? '').toContain('cannot be changed here')
    for (const label of ['Assign Role', 'Resend invite', 'Reset', 'Suspend', 'Reactivate']) {
      expect(sheet.textContent ?? '').not.toContain(label)
    }
  })

  it('offers no Role assignment on the root account', async () => {
    await mountAccess()
    const sheet = await openRootSheet()
    expect(sheet.textContent ?? '').toContain('No assignments.')
    expect(sheet.textContent ?? '').not.toContain('Assign a Role.')
  })

  it('offers no delete action for any user', async () => {
    await mountAccess()
    const table = document.querySelector('tbody')
    expect(table?.textContent ?? '').not.toContain('Delete')
    const sheet = await openRootSheet()
    expect(sheet.textContent ?? '').not.toContain('Delete')
  })
})
