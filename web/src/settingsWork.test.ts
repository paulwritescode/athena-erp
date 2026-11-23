// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SettingsPage from './pages/SettingsPage.vue'
import { eventClient, jobClient } from './api'

vi.mock('./api', () => ({
  eventClient: { listOutbox: vi.fn() },
  jobClient: { listJobs: vi.fn() },
  workspaceClient: { getIdentity: vi.fn(), setIdentity: vi.fn(), getAppearance: vi.fn(), setAppearance: vi.fn() },
  authClient: { getSecuritySettings: vi.fn(), setSecuritySettings: vi.fn() },
  systemClient: {},
  ledgerClient: {},
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function settle() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 20))
  await nextTick()
}

async function mountPage(permissions: string[]) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SettingsPage, { permissions }) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await settle()
}

describe('SettingsPage background work', () => {
  it('lists recent failures for builders with a way to open Jobs', async () => {
    vi.mocked(jobClient.listJobs).mockResolvedValue({
      jobs: [{ id: 'j1', kind: 'probe-a', status: 'dead', lastError: 'boom' }], nextPageToken: '',
    } as never)
    vi.mocked(eventClient.listOutbox).mockResolvedValue({
      rows: [{ id: 'o1', eventType: 'posting.posted', status: 'dead', attempts: 5, lastError: '' }], nextPageToken: '',
    } as never)
    await mountPage(['build'])
    expect(vi.mocked(jobClient.listJobs)).toHaveBeenCalledWith({ status: 'dead', pageSize: 5 })
    expect(vi.mocked(eventClient.listOutbox)).toHaveBeenCalledWith({ status: 'dead', pageSize: 5 })
    expect(document.body.textContent).toContain('Background work')
    expect(document.body.textContent).toContain('probe-a')
    expect(document.body.textContent).toContain('boom')
    expect(document.body.textContent).toContain('stopped after 5 tries')
    expect([...document.querySelectorAll('button')].some((b) => b.textContent?.includes('Open Jobs'))).toBe(true)
  })

  it('shows the all-clear when nothing failed', async () => {
    vi.mocked(jobClient.listJobs).mockResolvedValue({ jobs: [], nextPageToken: '' } as never)
    vi.mocked(eventClient.listOutbox).mockResolvedValue({ rows: [], nextPageToken: '' } as never)
    await mountPage(['build'])
    expect(document.body.textContent).toContain('Everything ran. No failed jobs or deliveries.')
  })

  it('stays hidden without the build permission and loads nothing', async () => {
    await mountPage([])
    expect(document.body.textContent).not.toContain('Background work')
    expect(vi.mocked(jobClient.listJobs)).not.toHaveBeenCalled()
    expect(vi.mocked(eventClient.listOutbox)).not.toHaveBeenCalled()
  })
})
