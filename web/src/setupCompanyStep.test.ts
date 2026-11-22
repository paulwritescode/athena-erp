// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SetupCompanyStep from './components/setup/SetupCompanyStep.vue'
import { ledgerClient } from './api'

vi.mock('./api', () => ({
  ledgerClient: { listCompanies: vi.fn(), saveCompany: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function mountStep(
  props: { initialName: string; canRead: boolean; canWrite: boolean; showBack?: boolean },
  onDone: (value: unknown) => void,
) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SetupCompanyStep, { ...props, onDone }) })
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

describe('SetupCompanyStep', () => {
  it('creates the first company', async () => {
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [] } as never)
    vi.mocked(ledgerClient.saveCompany).mockResolvedValue({ company: { name: 'acme', label: 'Acme Trading' } } as never)
    const done: unknown[] = []
    await mountStep({ initialName: '', canRead: true, canWrite: true }, (value) => done.push(value))
    await setInput('setup-company-code', 'acme')
    await setInput('setup-company-name', 'Acme Trading')
    button('Save company & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const save = vi.mocked(ledgerClient.saveCompany)
    expect(save).toHaveBeenCalledTimes(1)
    expect(save.mock.calls[0][0]).toMatchObject({ company: { name: 'acme', label: 'Acme Trading', baseCurrency: '' } })
    expect(done).toEqual([{ name: 'acme', label: 'Acme Trading' }])
  })

  it('rejects a bad code inline without saving', async () => {
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [] } as never)
    const done: unknown[] = []
    await mountStep({ initialName: '', canRead: true, canWrite: true }, (value) => done.push(value))
    await setInput('setup-company-code', 'Acme!')
    await setInput('setup-company-name', 'Acme')
    button('Save company & continue').click()
    await nextTick()
    expect(document.body.textContent).toContain('short code')
    expect(vi.mocked(ledgerClient.saveCompany)).not.toHaveBeenCalled()
    expect(done).toEqual([])
  })

  it('continues with an existing company', async () => {
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({
      companies: [{ name: 'acme', label: 'Acme Trading', disabled: false }],
    } as never)
    const done: unknown[] = []
    await mountStep({ initialName: '', canRead: true, canWrite: true }, (value) => done.push(value))
    expect(document.body.textContent).toContain('Continue with Acme Trading')
    button('Continue with Acme Trading').click()
    await nextTick()
    expect(vi.mocked(ledgerClient.saveCompany)).not.toHaveBeenCalled()
    expect(done).toEqual([{ name: 'acme', label: 'Acme Trading' }])
  })

  it('hides Back on the first step', async () => {
    vi.mocked(ledgerClient.listCompanies).mockResolvedValue({ companies: [] } as never)
    await mountStep({ initialName: '', canRead: true, canWrite: true, showBack: false }, () => {})
    expect([...document.querySelectorAll('button')].some((b) => b.textContent?.includes('Back'))).toBe(false)
  })

  it('explains without the read permission and moves on', async () => {
    const done: unknown[] = []
    await mountStep({ initialName: '', canRead: false, canWrite: false }, (value) => done.push(value))
    expect(document.body.textContent).toContain('You need the books permission to see companies.')
    expect(vi.mocked(ledgerClient.listCompanies)).not.toHaveBeenCalled()
    button('Continue').click()
    await nextTick()
    expect(done).toEqual([null])
  })
})
