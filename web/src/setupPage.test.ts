// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SetupPage from './pages/SetupPage.vue'
import { currentMonthBounds, firstPeriodName, fiscalYearName } from './components/setup/setupModel'
import { authClient, ledgerClient, workspaceClient } from './api'

vi.mock('./api', () => ({
  authClient: { listUsers: vi.fn(), inviteUser: vi.fn() },
  ledgerClient: {
    listCompanies: vi.fn(),
    listCurrencies: vi.fn(),
    listPeriods: vi.fn(),
    listFiscalYears: vi.fn(),
    saveCompany: vi.fn(),
    saveCurrency: vi.fn(),
    saveFiscalYear: vi.fn(),
    savePeriod: vi.fn(),
    getReadiness: vi.fn(),
  },
  workspaceClient: { getIdentity: vi.fn(), setIdentity: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function mountPage(permissions: string[], onNavigate: (page: unknown) => void) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SetupPage, { permissions, onNavigate }) })
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

function heading(): string {
  return document.querySelector('h2')?.textContent ?? ''
}

async function settle() {
  // The step transition rides requestAnimationFrame in jsdom; out-in
  // keeps the old step mounted until the frame lands.
  await new Promise((resolve) => setTimeout(resolve, 100))
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

const fullPermissions = ['build', 'ledger:read', 'ledger:write', 'settings:workspace', 'access:users']
const blankIdentity = { displayName: 'Acme', logoLightUrl: '', logoDarkUrl: '', favicon32Url: '', favicon180Url: '' }

describe('SetupPage', () => {
  it('walks an empty workspace from company to ready', async () => {
    const company = { name: 'acme', label: 'Acme Trading', baseCurrency: '', disabled: false, revision: BigInt(1) }
    vi.mocked(ledgerClient.listCompanies)
      .mockResolvedValueOnce({ companies: [] } as never)
      .mockResolvedValueOnce({ companies: [] } as never)
      .mockResolvedValue({ companies: [{ ...company }] } as never)
    vi.mocked(ledgerClient.listCurrencies).mockResolvedValue({ currencies: [] } as never)
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(ledgerClient.listFiscalYears).mockResolvedValue({ fiscalYears: [] } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...blankIdentity } } as never)
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [] } as never)
    vi.mocked(ledgerClient.saveCompany)
      .mockResolvedValueOnce({ company: { name: 'acme', label: 'Acme Trading' } } as never)
      .mockResolvedValueOnce({ company: { name: 'acme', label: 'Acme Trading', baseCurrency: 'USD' } } as never)
    vi.mocked(ledgerClient.saveCurrency).mockResolvedValue({ currency: { code: 'USD' } } as never)
    vi.mocked(ledgerClient.saveFiscalYear).mockResolvedValue({} as never)
    vi.mocked(ledgerClient.savePeriod).mockResolvedValue({ period: { label: 'Walk Period' } } as never)
    vi.mocked(authClient.inviteUser).mockResolvedValue({ user: { username: 'alice' }, challenge: 'code-1' } as never)
    vi.mocked(ledgerClient.getReadiness).mockResolvedValue({
      checks: [{ name: 'Account types available', ok: false, detail: 'save one with: ...' }],
    } as never)
    const navigate: unknown[] = []
    await mountPage(fullPermissions, (page) => navigate.push(page))

    expect(heading()).toContain('Which company is this setup for?')
    await setInput('setup-company-code', 'acme')
    await setInput('setup-company-name', 'Acme Trading')
    button('Save company & continue').click()
    await settle()
    expect(heading()).toContain('Add the workspace logo')
    button('Skip for now').click()
    await settle()
    expect(heading()).toContain('Choose the base money')
    await setInput('setup-currency-code', 'USD')
    await setInput('setup-currency-name', 'US dollar')
    button('Save USD & continue').click()
    await settle()
    expect(heading()).toContain('Open the first period')
    button('Open first period & continue').click()
    await settle()
    expect(heading()).toContain('Invite the first owner')
    await setInput('setup-owner-username', 'alice')
    button('Invite owner & continue').click()
    await settle()
    expect(document.body.textContent).toContain('code-1')
    button('Continue').click()
    await settle()
    expect(heading()).toContain('Is the workspace ready?')

    // The period and year carry the derived names.
    const bounds = currentMonthBounds()
    expect(vi.mocked(ledgerClient.saveFiscalYear).mock.calls[0][0]).toMatchObject({
      fiscalYear: { name: fiscalYearName(bounds.start), company: 'acme' },
    })
    expect(vi.mocked(ledgerClient.savePeriod).mock.calls[0][0]).toMatchObject({
      period: { name: firstPeriodName(bounds.start), book: 'main', fiscalYear: fiscalYearName(bounds.start) },
    })
    // The checklist reads the walked state.
    expect(document.body.textContent).toContain('Acme Trading (acme)')
    expect(document.body.textContent).toContain('Logo set · skipped')
    expect(document.body.textContent).toContain('Walk Period')
    expect(document.body.textContent).toContain('alice')
    expect(document.body.textContent).toContain('Account types available · missing')
    button('Open overview').click()
    await nextTick()
    expect(navigate).toEqual(['overview'])
  })

  it('lands on ready when everything is already set up', async () => {
    const now = new Date()
    const start = new Date(now.getFullYear(), now.getMonth() - 1, 1)
    const end = new Date(now.getFullYear(), now.getMonth() + 2, 0)
    const iso = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({
      companies: [{ name: 'acme', label: 'Acme Trading', baseCurrency: 'USD', disabled: false, revision: BigInt(2) }],
    } as never)
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({
      periods: [{ name: 'p', label: 'Wide Open', startDate: iso(start), endDate: iso(end), status: 'open' }],
    } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({
      identity: { ...blankIdentity, logoLightUrl: '/brand/logo-aaa.png' },
    } as never)
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [{ username: 'alice', owner: true }] } as never)
    vi.mocked(ledgerClient.getReadiness).mockResolvedValue({
      checks: [{ name: 'Account types available', ok: true, detail: '' }],
    } as never)
    await mountPage(fullPermissions, () => {})
    expect(heading()).toContain('Is the workspace ready?')
    expect(document.body.textContent).toContain('Everything is ready.')
  })

  it('goes back to the previous step', async () => {
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({
      companies: [{ name: 'acme', label: 'Acme Trading', baseCurrency: '', disabled: false, revision: BigInt(1) }],
    } as never)
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...blankIdentity } } as never)
    vi.mocked(authClient.listUsers).mockResolvedValue({ users: [] } as never)
    await mountPage(fullPermissions, () => {})
    expect(heading()).toContain('Add the workspace logo')
    button('Back').click()
    await settle()
    expect(heading()).toContain('Which company is this setup for?')
  })

  it('denies a builder with none of the setup permissions', async () => {
    await mountPage(['build'], () => {})
    expect(document.body.textContent).toContain("You don't have access to Setup")
    expect(vi.mocked(ledgerClient.listCompanies)).not.toHaveBeenCalled()
    expect(vi.mocked(workspaceClient.getIdentity)).not.toHaveBeenCalled()
  })
})
