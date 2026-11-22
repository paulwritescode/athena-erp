// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SetupReadinessStep from './components/setup/SetupReadinessStep.vue'
import { ledgerClient, workspaceClient } from './api'
import type { SetupSnapshot, SetupStepId } from './components/setup/setupModel'

vi.mock('./api', () => ({
  ledgerClient: { getReadiness: vi.fn() },
  workspaceClient: { getIdentity: vi.fn(), setIdentity: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function mountStep(
  props: { snapshot: SetupSnapshot; skipped: ReadonlySet<SetupStepId>; canReadLedger: boolean; canEditIdentity: boolean },
  events: { onGoto?: (step: unknown) => void; onNavigate?: (page: unknown) => void },
) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SetupReadinessStep, { ...props, ...events }) })
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

const full: SetupSnapshot = {
  companyName: 'acme',
  companyLabel: 'Acme Trading',
  baseCurrency: 'USD',
  hasLogo: true,
  openPeriodLabel: 'November 2026',
  openPeriodDates: 'Nov 1 – Nov 30',
  ownerNames: ['alice'],
}

const identity = { displayName: 'Acme', logoLightUrl: '/brand/logo-aaa.png', logoDarkUrl: '', favicon32Url: '', favicon180Url: '' }

describe('SetupReadinessStep', () => {
  it('links each missing check back to its step', async () => {
    vi.mocked(ledgerClient.getReadiness).mockResolvedValue({ checks: [] } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity } as never)
    const goto: unknown[] = []
    await mountStep(
      {
        snapshot: { ...full, baseCurrency: '', ownerNames: [] },
        skipped: new Set(),
        canReadLedger: true,
        canEditIdentity: true,
      },
      { onGoto: (step) => goto.push(step) },
    )
    expect(document.body.textContent).toContain('Base money chosen · missing')
    expect(document.body.textContent).toContain('Owner invited · missing')
    button('Go to Money').click()
    await nextTick()
    expect(goto).toEqual(['currency'])
    button('Continue setup').click()
    await nextTick()
    expect(goto).toEqual(['currency', 'currency'])
  })

  it('shows the books checks it does not cover, with a way to fix them', async () => {
    vi.mocked(ledgerClient.getReadiness).mockResolvedValue({
      checks: [
        { name: 'Company set up', ok: true, detail: '' },
        { name: 'Account types available', ok: false, detail: 'save one with: ...' },
        { name: 'Posting accounts available', ok: true, detail: '' },
      ],
    } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity } as never)
    const navigate: unknown[] = []
    await mountStep(
      { snapshot: full, skipped: new Set(), canReadLedger: true, canEditIdentity: true },
      { onNavigate: (page) => navigate.push(page) },
    )
    // The wizard covers Company itself, so the server copy stays out: one row.
    expect(document.body.textContent?.match(/Company set up/g) ?? []).toHaveLength(1)
    expect(document.body.textContent).toContain('Account types available · missing')
    button('Open Ledger setup').click()
    await nextTick()
    expect(navigate).toEqual(['ledger'])
  })

  it('finishes to the overview when everything is ready', async () => {
    vi.mocked(ledgerClient.getReadiness).mockResolvedValue({
      checks: [
        { name: 'Account types available', ok: true, detail: '' },
        { name: 'Posting accounts available', ok: true, detail: '' },
        { name: 'Posting rules available', ok: true, detail: '' },
      ],
    } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity } as never)
    const navigate: unknown[] = []
    await mountStep(
      { snapshot: full, skipped: new Set(), canReadLedger: true, canEditIdentity: true },
      { onNavigate: (page) => navigate.push(page) },
    )
    expect(document.body.textContent).toContain('Everything is ready.')
    // The B5 identity section renders for review.
    expect((document.querySelector('#identity-name') as HTMLInputElement).value).toBe('Acme')
    button('Open overview').click()
    await nextTick()
    expect(navigate).toEqual(['overview'])
  })

  it('marks skipped steps skipped', async () => {
    vi.mocked(ledgerClient.getReadiness).mockResolvedValue({ checks: [] } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity } as never)
    await mountStep(
      {
        snapshot: { ...full, hasLogo: false, ownerNames: [] },
        skipped: new Set(['logo', 'invite']),
        canReadLedger: true,
        canEditIdentity: true,
      },
      {},
    )
    expect(document.body.textContent).toContain('Logo set · skipped')
    expect(document.body.textContent).toContain('Owner invited · skipped')
  })
})
