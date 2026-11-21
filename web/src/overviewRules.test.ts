// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import OverviewPage from './pages/OverviewPage.vue'
import { ruleClient } from './api'

vi.mock('./api', () => ({ ruleClient: { listRules: vi.fn() } }))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

function rulesFooter(): string {
  const cards = document.querySelectorAll('.foundation-card .card-footer .muted-label')
  return cards[2]?.textContent ?? '(missing)'
}

async function mountOverview(): Promise<void> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(OverviewPage, { recordTypes: [], databaseState: 'Connected', apiState: 'ok' })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

describe('OverviewPage Rules card', () => {
  it('shows the live Rule count', async () => {
    vi.mocked(ruleClient.listRules).mockResolvedValue({ rules: [{}, {}] } as never)
    await mountOverview()
    expect(rulesFooter()).toBe('2 rules')
  })

  it('shows No rules yet when the list is empty', async () => {
    vi.mocked(ruleClient.listRules).mockResolvedValue({ rules: [] } as never)
    await mountOverview()
    expect(rulesFooter()).toBe('No rules yet')
  })

  it('omits the count when the list cannot be read', async () => {
    vi.mocked(ruleClient.listRules).mockRejectedValue(new Error('denied'))
    await mountOverview()
    expect(rulesFooter()).toBe('')
  })
})
