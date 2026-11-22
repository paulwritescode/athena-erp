// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import AccessPage from './pages/AccessPage.vue'
import { authClient } from './api'
import { gloss } from './copy/glossary'

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

const rootReason = gloss('rootAdministrator').explanation

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

// Root administrator desk tests (ADR-0018, task B8 slice 2): the root row
// carries the caption and a lock badge with the reason, and its actions
// stay visible but disabled with the reason.
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
  it('shows the caption and a lock badge with the reason on the root row', async () => {
    await mountAccess()
    const rows = [...document.querySelectorAll('tbody tr')]
    const rootRow = rows[0] as HTMLElement
    const amyRow = rows[1] as HTMLElement
    expect(rootRow.textContent ?? '').toContain('Root administrator')
    expect(amyRow.textContent ?? '').not.toContain('Root administrator')
    const badge = [...rootRow.querySelectorAll('div[data-slot="badge"]')].find((el) => el.textContent === 'Root')
    expect(badge).not.toBeUndefined()
    expect(badge?.querySelector('svg')).not.toBeNull()
    expect(badge?.getAttribute('title')).toBe(rootReason)
  })

  it('disables the root row actions with the reason', async () => {
    await mountAccess()
    // Both rows get the row-actions menu now; the root menu stays disabled.
    const menus = [...document.querySelectorAll('button[aria-label^="Actions for row"]')]
    expect(menus).toHaveLength(2)
    ;(menus[0] as HTMLElement).click()
    await flush()
    const items = [...document.querySelectorAll('[role="menuitem"]')]
    expect(items.length).toBeGreaterThan(0)
    for (const item of items) {
      expect(item.getAttribute('aria-disabled') ?? item.getAttribute('data-disabled')).toBeTruthy()
      expect(item.getAttribute('title')).toBe(rootReason)
    }
    expect(items.map((item) => item.textContent?.trim()).join(' ')).toContain('Suspend')
  })

  it('disables the root sheet actions with the reason', async () => {
    await mountAccess()
    const sheet = await openRootSheet()
    expect(sheet.textContent ?? '').toContain('cannot be changed here')
    const buttons = [...sheet.querySelectorAll('button')].filter((button) => ['Assign Role', 'Reset', 'Suspend'].some((label) => button.textContent?.includes(label)))
    expect(buttons.length).toBeGreaterThan(0)
    for (const button of buttons) {
      expect(button.disabled).toBe(true)
      expect(button.getAttribute('aria-label') ?? '').toContain(rootReason)
    }
  })

  it('offers no Role assignment or delete action for any user', async () => {
    await mountAccess()
    const table = document.querySelector('tbody')
    expect(table?.textContent ?? '').not.toContain('Delete')
    const sheet = await openRootSheet()
    expect(sheet.textContent ?? '').toContain('No assignments.')
    expect(sheet.textContent ?? '').not.toContain('Delete')
  })
})
