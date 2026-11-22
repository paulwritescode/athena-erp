// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import SetupPeriodStep from './components/setup/SetupPeriodStep.vue'
import { currentMonthBounds, firstPeriodLabel, firstPeriodName, fiscalYearName } from './components/setup/setupModel'
import { ledgerClient } from './api'

vi.mock('./api', () => ({
  ledgerClient: { listPeriods: vi.fn(), listFiscalYears: vi.fn(), saveFiscalYear: vi.fn(), savePeriod: vi.fn() },
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
  const app = createApp({ setup: () => () => h(SetupPeriodStep, { ...props, onDone }) })
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

const props = { companyName: 'acme', companyLabel: 'Acme Trading', canRead: true, canWrite: true }

describe('SetupPeriodStep', () => {
  it('opens the first period with its financial year', async () => {
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(ledgerClient.listFiscalYears).mockResolvedValue({ fiscalYears: [] } as never)
    vi.mocked(ledgerClient.saveFiscalYear).mockResolvedValue({} as never)
    vi.mocked(ledgerClient.savePeriod).mockResolvedValue({ period: { label: 'Test Period' } } as never)
    const done: unknown[] = []
    await mountStep(props, (value) => done.push(value))
    button('Open first period & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const bounds = currentMonthBounds()
    const year = fiscalYearName(bounds.start)
    const saveYear = vi.mocked(ledgerClient.saveFiscalYear)
    expect(saveYear).toHaveBeenCalledTimes(1)
    expect(saveYear.mock.calls[0][0]).toMatchObject({
      fiscalYear: { name: year, label: year, company: 'acme', startDate: `${year}-01-01`, endDate: `${year}-12-31` },
    })
    const savePeriod = vi.mocked(ledgerClient.savePeriod)
    expect(savePeriod).toHaveBeenCalledTimes(1)
    expect(savePeriod.mock.calls[0][0]).toMatchObject({
      period: {
        company: 'acme',
        book: 'main',
        name: firstPeriodName(bounds.start),
        label: firstPeriodLabel(bounds.start),
        fiscalYear: year,
        startDate: bounds.start,
        endDate: bounds.end,
      },
    })
    expect(done).toEqual([{ label: 'Test Period', dates: expect.stringContaining('–') }])
  })

  it('reuses the financial year when it already exists', async () => {
    const bounds = currentMonthBounds()
    const year = fiscalYearName(bounds.start)
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(ledgerClient.listFiscalYears).mockResolvedValue({
      fiscalYears: [{ name: year, label: year, company: 'acme' }],
    } as never)
    vi.mocked(ledgerClient.savePeriod).mockResolvedValue({ period: { label: 'Test Period' } } as never)
    const done: unknown[] = []
    await mountStep(props, (value) => done.push(value))
    button('Open first period & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(vi.mocked(ledgerClient.saveFiscalYear)).not.toHaveBeenCalled()
    expect(vi.mocked(ledgerClient.savePeriod)).toHaveBeenCalledTimes(1)
    expect(done).toEqual([{ label: 'Test Period', dates: expect.stringContaining('–') }])
  })

  it('continues when a period already covers today', async () => {
    const bounds = currentMonthBounds()
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({
      periods: [{ name: 'p', label: 'Open Month', startDate: bounds.start, endDate: bounds.end, status: 'open' }],
    } as never)
    vi.mocked(ledgerClient.listFiscalYears).mockResolvedValue({ fiscalYears: [] } as never)
    const done: unknown[] = []
    await mountStep(props, (value) => done.push(value))
    expect(document.body.textContent).toContain('Open Month is open')
    button('Continue').click()
    await nextTick()
    expect(vi.mocked(ledgerClient.savePeriod)).not.toHaveBeenCalled()
    expect(done).toEqual([{ label: 'Open Month', dates: expect.stringContaining('–') }])
  })

  it('retries in place after a failed period save (B1)', async () => {
    const bounds = currentMonthBounds()
    const year = fiscalYearName(bounds.start)
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(ledgerClient.listFiscalYears).mockResolvedValue({ fiscalYears: [] } as never)
    vi.mocked(ledgerClient.saveFiscalYear).mockResolvedValue({
      fiscalYear: { name: year, label: year, company: 'acme', startDate: `${year}-01-01`, endDate: `${year}-12-31` },
    } as never)
    vi.mocked(ledgerClient.savePeriod)
      .mockRejectedValueOnce(new ConnectError('overlaps early', Code.InvalidArgument))
      .mockResolvedValueOnce({ period: { label: 'Retried Period' } } as never)
    const done: unknown[] = []
    await mountStep(props, (value) => done.push(value))
    button('Open first period & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(document.body.textContent).toContain('overlaps early')
    expect(done).toEqual([])
    // Unchanged retry: the year is reused, not re-saved.
    button('Open first period & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(vi.mocked(ledgerClient.saveFiscalYear)).toHaveBeenCalledTimes(1)
    expect(vi.mocked(ledgerClient.savePeriod)).toHaveBeenCalledTimes(2)
    expect(done).toEqual([{ label: 'Retried Period', dates: expect.stringContaining('–') }])
  })

  it('treats a duplicate year as existing and carries on', async () => {
    const bounds = currentMonthBounds()
    const year = fiscalYearName(bounds.start)
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(ledgerClient.listFiscalYears)
      .mockResolvedValueOnce({ fiscalYears: [] } as never)
      .mockResolvedValueOnce({
        fiscalYears: [{ name: year, label: year, company: 'acme', startDate: `${year}-01-01`, endDate: `${year}-12-31` }],
      } as never)
    vi.mocked(ledgerClient.saveFiscalYear).mockRejectedValueOnce(new ConnectError(`fiscal year "${year}" already exists`, Code.AlreadyExists))
    vi.mocked(ledgerClient.savePeriod).mockResolvedValue({ period: { label: 'Adopted Period' } } as never)
    const done: unknown[] = []
    await mountStep(props, (value) => done.push(value))
    button('Open first period & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(vi.mocked(ledgerClient.savePeriod)).toHaveBeenCalledTimes(1)
    expect(done).toEqual([{ label: 'Adopted Period', dates: expect.stringContaining('–') }])
  })

  it('shows a server overlap inline', async () => {
    vi.mocked(ledgerClient.listPeriods).mockResolvedValue({ periods: [] } as never)
    vi.mocked(ledgerClient.listFiscalYears).mockResolvedValue({ fiscalYears: [] } as never)
    vi.mocked(ledgerClient.saveFiscalYear).mockResolvedValue({} as never)
    vi.mocked(ledgerClient.savePeriod).mockRejectedValue(new ConnectError('overlaps nov-1', Code.InvalidArgument))
    const done: unknown[] = []
    await mountStep(props, (value) => done.push(value))
    button('Open first period & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(document.body.textContent).toContain('overlaps nov-1')
    expect(done).toEqual([])
  })
})
