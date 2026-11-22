// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SetupCurrencyStep from './components/setup/SetupCurrencyStep.vue'
import { ledgerClient } from './api'

vi.mock('./api', () => ({
  ledgerClient: { listCurrencies: vi.fn(), listCompanies: vi.fn(), saveCurrency: vi.fn(), saveCompany: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function mountStep(props: { companyName: string; companyLabel: string; canRead: boolean; canWrite: boolean }, onDone: (value: unknown) => void) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SetupCurrencyStep, { ...props, onDone }) })
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

const company = { name: 'acme', label: 'Acme Trading', baseCurrency: '', disabled: false, revision: BigInt(1) }

describe('SetupCurrencyStep', () => {
  it('saves new money and sets it as the base', async () => {
    vi.mocked(ledgerClient.listCurrencies).mockResolvedValue({ currencies: [] } as never)
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [company] } as never)
    vi.mocked(ledgerClient.saveCurrency).mockResolvedValue({ currency: { code: 'USD', label: 'US dollar', exponent: 2 } } as never)
    vi.mocked(ledgerClient.saveCompany).mockResolvedValue({ company: { ...company, baseCurrency: 'USD' } } as never)
    const done: unknown[] = []
    await mountStep({ companyName: 'acme', companyLabel: 'Acme Trading', canRead: true, canWrite: true }, (value) => done.push(value))
    await setInput('setup-currency-code', 'usd')
    await setInput('setup-currency-name', 'US dollar')
    button('Save USD & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const save = vi.mocked(ledgerClient.saveCurrency)
    expect(save).toHaveBeenCalledTimes(1)
    expect(save.mock.calls[0][0]).toMatchObject({ currency: { code: 'USD', label: 'US dollar', exponent: 2 } })
    const base = vi.mocked(ledgerClient.saveCompany)
    expect(base).toHaveBeenCalledTimes(1)
    expect(base.mock.calls[0][0]).toMatchObject({ company: { name: 'acme', baseCurrency: 'USD' }, expectedRevision: BigInt(1) })
    expect(done).toEqual([{ baseCurrency: 'USD' }])
  })

  it('uses saved money without saving again', async () => {
    vi.mocked(ledgerClient.listCurrencies).mockResolvedValue({
      currencies: [{ code: 'USD', label: 'US dollar', exponent: 2, disabled: false }],
    } as never)
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [company] } as never)
    vi.mocked(ledgerClient.saveCompany).mockResolvedValue({ company: { ...company, baseCurrency: 'USD' } } as never)
    const done: unknown[] = []
    await mountStep({ companyName: 'acme', companyLabel: 'Acme Trading', canRead: true, canWrite: true }, (value) => done.push(value))
    button('Use USD & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(vi.mocked(ledgerClient.saveCurrency)).not.toHaveBeenCalled()
    expect(vi.mocked(ledgerClient.saveCompany)).toHaveBeenCalledTimes(1)
    expect(done).toEqual([{ baseCurrency: 'USD' }])
  })

  it('continues when the base money is already set', async () => {
    vi.mocked(ledgerClient.listCurrencies).mockResolvedValue({ currencies: [] } as never)
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [{ ...company, baseCurrency: 'USD' }] } as never)
    const done: unknown[] = []
    await mountStep({ companyName: 'acme', companyLabel: 'Acme Trading', canRead: true, canWrite: true }, (value) => done.push(value))
    expect(document.body.textContent).toContain('USD is the base money')
    button('Continue').click()
    await nextTick()
    expect(vi.mocked(ledgerClient.saveCurrency)).not.toHaveBeenCalled()
    expect(vi.mocked(ledgerClient.saveCompany)).not.toHaveBeenCalled()
    expect(done).toEqual([{ baseCurrency: 'USD' }])
  })

  it('rejects a bad code inline without saving', async () => {
    vi.mocked(ledgerClient.listCurrencies).mockResolvedValue({ currencies: [] } as never)
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [company] } as never)
    const done: unknown[] = []
    await mountStep({ companyName: 'acme', companyLabel: 'Acme Trading', canRead: true, canWrite: true }, (value) => done.push(value))
    await setInput('setup-currency-code', 'US')
    await setInput('setup-currency-name', 'US dollar')
    button('Save US & continue').click()
    await nextTick()
    expect(document.body.textContent).toContain('three capital letters')
    expect(vi.mocked(ledgerClient.saveCurrency)).not.toHaveBeenCalled()
    expect(done).toEqual([])
  })
})
