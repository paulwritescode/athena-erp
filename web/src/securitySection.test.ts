// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SecuritySection from './components/settings/SecuritySection.vue'
import { authClient } from './api'

vi.mock('./api', () => ({
  authClient: { getSecuritySettings: vi.fn(), setSecuritySettings: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

function setting(key: string, value: number, extra: Record<string, unknown> = {}) {
  return { key, value, defaultValue: 30, minValue: 5, maxValue: 480, unit: 'minutes', description: 'How long.', ...extra }
}

async function mountSection(permissions: string[]) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SecuritySection, { permissions }) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

async function setInput(id: string, value: string) {
  const input = document.querySelector(`#${CSS.escape(id)}`) as HTMLInputElement | null
  if (!input) throw new Error(`missing ${id}`)
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

describe('SecuritySection', () => {
  it('shows a placeholder when the server client predates the RPC', async () => {
    const client = authClient as unknown as Record<string, unknown>
    const get = client.getSecuritySettings
    const set = client.setSecuritySettings
    delete client.getSecuritySettings
    try {
      await mountSection(['settings:security'])
      expect(document.body.textContent).toContain('need a newer server')
    } finally {
      client.getSecuritySettings = get
      client.setSecuritySettings = set
    }
  })

  it('lists settings with bounds and saves only changed values', async () => {
    vi.mocked(authClient.getSecuritySettings).mockResolvedValue({
      settings: [setting('security.session.idle_minutes', 30), setting('security.session.absolute_hours', 12)],
    } as never)
    vi.mocked(authClient.setSecuritySettings).mockResolvedValue({ settings: [] } as never)
    await mountSection(['settings:security'])
    expect(document.body.textContent).toContain('Idle sign-out')
    expect(document.body.textContent).toContain('Between 5 and 480 minutes; default 30.')
    await setInput('security-security.session.idle_minutes', '45')
    document.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(authClient.setSecuritySettings).toHaveBeenCalledWith({ settings: { 'security.session.idle_minutes': 45 } })
  })

  it('blocks out-of-bounds values with an inline error', async () => {
    vi.mocked(authClient.getSecuritySettings).mockResolvedValue({ settings: [setting('security.session.idle_minutes', 30)] } as never)
    await mountSection(['settings:security'])
    await setInput('security-security.session.idle_minutes', '4')
    expect(document.body.textContent).toContain('Between 5 and 480.')
    document.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(authClient.setSecuritySettings).not.toHaveBeenCalled()
  })

  it('is read-only without the permission', async () => {
    vi.mocked(authClient.getSecuritySettings).mockResolvedValue({ settings: [setting('security.session.idle_minutes', 30)] } as never)
    await mountSection(['access:sessions'])
    expect(document.body.textContent).toContain('need the security permission')
    expect(document.querySelector('input')?.disabled).toBe(true)
    expect([...document.querySelectorAll('button')].some((b) => b.textContent?.includes('Save security'))).toBe(false)
  })
})
