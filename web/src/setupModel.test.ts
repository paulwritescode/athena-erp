// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import {
  buildLedgerChecks,
  buildWizardChecks,
  checkCurrencyCode,
  checkDate,
  checkDateOrder,
  checkDisplayName,
  checkExponent,
  checkLabel,
  checkLedgerName,
  checkUsername,
  coversToday,
  currentMonthBounds,
  firstMissingStep,
  firstPeriodLabel,
  firstPeriodName,
  fiscalYearName,
  normalizeUsername,
  setupSteps,
  shortDateRange,
  setupStepWords,
  skippableSteps,
  yearBounds,
  type SetupSnapshot,
} from './components/setup/setupModel'

describe('steps', () => {
  it('lists company, logo, money, period, owner and ready in order', () => {
    expect(setupSteps.map((step) => step.id)).toEqual(['company', 'logo', 'currency', 'period', 'invite', 'ready'])
    expect(setupStepWords.currency).toBe('Money')
  })

  it('lets only logo and invite skip', () => {
    expect([...skippableSteps].sort()).toEqual(['invite', 'logo'])
  })
})

describe('field checks', () => {
  it('checks ledger codes like the server', () => {
    expect(checkLedgerName('Company', 'acme')).toBeNull()
    expect(checkLedgerName('Company', '9lives')).toBeNull()
    expect(checkLedgerName('Company', 'acme_trading-1')).toBeNull()
    expect(checkLedgerName('Company', '2026-11')).toBeNull()
    expect(checkLedgerName('Company', '')).toContain('short code')
    expect(checkLedgerName('Company', 'Acme')).toContain('short code')
    expect(checkLedgerName('Company', '-acme')).toContain('short code')
    expect(checkLedgerName('Company', 'a'.repeat(141))).toContain('short code')
    expect(checkLedgerName('Company', 'a'.repeat(140))).toBeNull()
    expect(checkLedgerName('Period', 'Bad Name')).toContain('Period uses')
  })

  it('checks labels like the server', () => {
    expect(checkLabel('Acme Trading')).toBeNull()
    expect(checkLabel('')).toContain('required')
    expect(checkLabel('a'.repeat(256))).toContain('255')
    expect(checkLabel('a'.repeat(255))).toBeNull()
  })

  it('checks currency codes like the server', () => {
    expect(checkCurrencyCode('USD')).toBeNull()
    expect(checkCurrencyCode('usd')).toContain('three capital letters')
    expect(checkCurrencyCode('US')).toContain('three capital letters')
    expect(checkCurrencyCode('USDD')).toContain('three capital letters')
    expect(checkCurrencyCode('U1D')).toContain('three capital letters')
  })

  it('checks decimal places like the server', () => {
    expect(checkExponent('2')).toBeNull()
    expect(checkExponent('0')).toBeNull()
    expect(checkExponent('12')).toBeNull()
    expect(checkExponent('')).toContain('0 to 12')
    expect(checkExponent('13')).toContain('0 to 12')
    expect(checkExponent('-1')).toContain('0 to 12')
    expect(checkExponent('2.5')).toContain('0 to 12')
    expect(checkExponent('two')).toContain('0 to 12')
  })

  it('checks display names like the server', () => {
    expect(checkDisplayName('')).toBeNull()
    expect(checkDisplayName('Alice')).toBeNull()
    expect(checkDisplayName('a'.repeat(256))).toContain('255')
    expect(checkDisplayName('a'.repeat(255))).toBeNull()
  })

  it('checks usernames like the server', () => {
    expect(normalizeUsername('  Alice_1 ')).toBe('alice_1')
    expect(checkUsername('alice')).toBeNull()
    expect(checkUsername('Alice.1-2_3')).toBeNull()
    expect(checkUsername('a')).toContain('2 to 63')
    expect(checkUsername('a'.repeat(64))).toContain('2 to 63')
    expect(checkUsername('alice doe')).toContain('2 to 63')
    expect(checkUsername("-alice")).toContain('2 to 63')
  })

  it('checks dates and their order like the server', () => {
    expect(checkDate('Start', '2026-11-01')).toBeNull()
    expect(checkDate('Start', '2026-13-01')).toContain('YYYY-MM-DD')
    expect(checkDate('Start', '2026-11-32')).toContain('YYYY-MM-DD')
    expect(checkDate('Start', 'Nov 1')).toContain('YYYY-MM-DD')
    expect(checkDateOrder('2026-11-01', '2026-11-30')).toBeNull()
    expect(checkDateOrder('2026-11-01', '2026-11-01')).toBeNull()
    expect(checkDateOrder('2026-11-30', '2026-11-01')).toContain('before the start')
  })
})

describe('derivations', () => {
  it('bounds the current month', () => {
    expect(currentMonthBounds(new Date(2026, 10, 22))).toEqual({ start: '2026-11-01', end: '2026-11-30' })
    expect(currentMonthBounds(new Date(2024, 1, 10))).toEqual({ start: '2024-02-01', end: '2024-02-29' })
    expect(currentMonthBounds(new Date(2026, 1, 10))).toEqual({ start: '2026-02-01', end: '2026-02-28' })
  })

  it('derives the year, period code and label from the start', () => {
    expect(yearBounds('2026-11-01')).toEqual({ start: '2026-01-01', end: '2026-12-31' })
    expect(fiscalYearName('2026-11-01')).toBe('2026')
    expect(firstPeriodName('2026-11-01')).toBe('2026-11')
    expect(firstPeriodLabel('2026-11-01')).toBe('November 2026')
    expect(shortDateRange('2026-11-01', '2026-11-30')).toBe('Nov 1 – Nov 30')
  })

  it('covers today only inside an open span', () => {
    expect(coversToday({ startDate: '2026-11-01', endDate: '2026-11-30', status: 'open' }, '2026-11-22')).toBe(true)
    expect(coversToday({ startDate: '2026-11-01', endDate: '2026-11-30', status: 'open' }, '2026-11-01')).toBe(true)
    expect(coversToday({ startDate: '2026-11-01', endDate: '2026-11-30', status: 'open' }, '2026-11-30')).toBe(true)
    expect(coversToday({ startDate: '2026-11-01', endDate: '2026-11-30', status: 'open' }, '2026-12-01')).toBe(false)
    expect(coversToday({ startDate: '2026-11-01', endDate: '2026-11-30', status: 'soft_closed' }, '2026-11-22')).toBe(false)
  })
})

const full: SetupSnapshot = {
  companyName: 'acme',
  companyLabel: 'Acme Trading',
  baseCurrency: 'USD',
  hasLogo: true,
  openPeriodLabel: 'November 2026',
  openPeriodDates: 'Nov 1 – Nov 30',
  ownerNames: ['alice'],
}

const empty: SetupSnapshot = {
  companyName: '',
  companyLabel: '',
  baseCurrency: '',
  hasLogo: false,
  openPeriodLabel: '',
  openPeriodDates: '',
  ownerNames: [],
}

describe('readiness', () => {
  it('marks every check ready on a full snapshot', () => {
    const checks = buildWizardChecks(full, new Set())
    expect(checks.map((check) => check.state)).toEqual(['ready', 'ready', 'ready', 'ready', 'ready'])
    expect(checks[0].detail).toContain('Acme Trading (acme)')
    expect(firstMissingStep(checks)).toBeNull()
  })

  it('marks every check missing on an empty snapshot', () => {
    const checks = buildWizardChecks(empty, new Set())
    expect(checks.map((check) => check.state)).toEqual(['missing', 'missing', 'missing', 'missing', 'missing'])
    expect(firstMissingStep(checks)).toBe('company')
  })

  it('marks skipped steps skipped, never missing', () => {
    const checks = buildWizardChecks(empty, new Set(['logo', 'invite']))
    expect(checks.map((check) => check.state)).toEqual(['missing', 'skipped', 'missing', 'missing', 'skipped'])
    expect(firstMissingStep(checks)).toBe('company')
  })

  it('points each check at its step', () => {
    const checks = buildWizardChecks(empty, new Set())
    expect(checks.map((check) => check.step)).toEqual(['company', 'logo', 'currency', 'period', 'invite'])
  })

  it('keeps only the server checks the wizard does not cover', () => {
    const server = [
      { name: 'Company set up', ok: true, detail: '' },
      { name: 'Base currency chosen', ok: true, detail: '' },
      { name: 'Currencies available', ok: true, detail: '' },
      { name: 'Today falls in an open period', ok: false, detail: 'save one with: ...' },
      { name: 'Account types available', ok: false, detail: 'save one with: ...' },
      { name: 'Posting accounts available', ok: true, detail: '' },
      { name: 'Posting rules available', ok: false, detail: 'save one with: ...' },
    ]
    expect(buildLedgerChecks(server).map((check) => check.name)).toEqual([
      'Account types available',
      'Posting accounts available',
      'Posting rules available',
    ])
  })

  it('keeps an unknown server check instead of dropping it', () => {
    const server = [{ name: 'Something new', ok: false, detail: 'do it' }]
    expect(buildLedgerChecks(server)).toEqual(server)
  })
})
