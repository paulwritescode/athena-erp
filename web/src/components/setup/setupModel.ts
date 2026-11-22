// Setup wizard model (B11 slice 2): step ids, field checks that mirror the
// server rules, derived names for the first financial year and period, and
// the readiness checklist. Pure: every rule here is covered in
// setupModel.test.ts, and the server checks the same again on every save.
export type SetupStepId = 'company' | 'logo' | 'currency' | 'period' | 'invite' | 'ready'

export const setupSteps: ReadonlyArray<{ id: SetupStepId; word: string }> = [
  { id: 'company', word: 'Company' },
  { id: 'logo', word: 'Logo' },
  { id: 'currency', word: 'Money' },
  { id: 'period', word: 'Period' },
  { id: 'invite', word: 'Owner' },
  { id: 'ready', word: 'Ready' },
]

export const setupStepWords: Record<SetupStepId, string> = Object.fromEntries(
  setupSteps.map((step) => [step.id, step.word]),
) as Record<SetupStepId, string>

// Steps the wizard may skip: a workspace works without a logo and the root
// account can invite owners later from Access.
export const skippableSteps: ReadonlySet<SetupStepId> = new Set(['logo', 'invite'])

// checkName in internal/ledger/ledger.go: company, period, book and fiscal
// year codes.
const ledgerNamePattern = /^[a-z0-9][a-z0-9_-]{0,139}$/

export function checkLedgerName(what: string, name: string): string | null {
  if (!ledgerNamePattern.test(name)) {
    return `${what} uses a short code: start with a lowercase letter or digit, then up to 139 lowercase letters, digits, underscores or hyphens.`
  }
  return null
}

// checkLabel in internal/ledger/ledger.go.
export function checkLabel(label: string): string | null {
  if (label === '' || [...label].length > 255) {
    return 'The name is required and holds at most 255 characters.'
  }
  return null
}

// types.NewCurrency in internal/types/money.go: three capitals.
export function checkCurrencyCode(code: string): string | null {
  if (!/^[A-Z]{3}$/.test(code)) {
    return 'The code is three capital letters, like USD.'
  }
  return null
}

// SaveCurrency in internal/ledger/currencies.go: 0 to 12.
export function checkExponent(raw: string): string | null {
  if (!/^\d{1,2}$/.test(raw.trim())) {
    return 'Decimal places is a whole number from 0 to 12.'
  }
  const value = Number(raw.trim())
  if (value < 0 || value > 12) {
    return 'Decimal places is a whole number from 0 to 12.'
  }
  return null
}

// validateNewUser in internal/auth/service.go: the display name is
// optional and holds at most 255 characters.
export function checkDisplayName(displayName: string): string | null {
  if ([...displayName].length > 255) {
    return 'The display name holds at most 255 characters.'
  }
  return null
}

// NormalizeUsername in internal/auth/service.go. The server lowercases and
// trims first; the wizard does the same so what it checks is what saves.
export function normalizeUsername(username: string): string {
  return username.trim().toLowerCase()
}

export function checkUsername(username: string): string | null {
  if (!/^[a-z0-9][a-z0-9._-]{1,62}$/.test(normalizeUsername(username))) {
    return 'A username has 2 to 63 characters: letters, digits, dot, dash and underscore, starting with a letter or digit.'
  }
  return null
}

// validDate and checkDateRange in internal/ledger/periods.go.
export function checkDate(what: string, value: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return `${what} is a date as YYYY-MM-DD.`
  }
  const month = value.slice(5, 7)
  const day = value.slice(8, 10)
  if (month < '01' || month > '12' || day < '01' || day > '31') {
    return `${what} is a date as YYYY-MM-DD.`
  }
  return null
}

export function checkDateOrder(start: string, end: string): string | null {
  if (end < start) {
    return 'The end date is before the start date.'
  }
  return null
}

function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

function toISODate(date: Date): string {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

/** First and last day of the month holding `at`, as YYYY-MM-DD. */
export function currentMonthBounds(at: Date = new Date()): { start: string; end: string } {
  const start = new Date(at.getFullYear(), at.getMonth(), 1)
  const end = new Date(at.getFullYear(), at.getMonth() + 1, 0)
  return { start: toISODate(start), end: toISODate(end) }
}

/** Calendar year holding the start date, as YYYY-MM-DD bounds. */
export function yearBounds(startDate: string): { start: string; end: string } {
  const year = startDate.slice(0, 4)
  return { start: `${year}-01-01`, end: `${year}-12-31` }
}

/** Financial year code derived from the period start: the year itself. */
export function fiscalYearName(startDate: string): string {
  return startDate.slice(0, 4)
}

/** First-period code derived from the period start: YYYY-MM. */
export function firstPeriodName(startDate: string): string {
  return startDate.slice(0, 7)
}

/** First-period label, like "November 2026". */
export function firstPeriodLabel(startDate: string): string {
  const [year, month] = startDate.split('-').map(Number)
  const name = new Date(year, month - 1, 1).toLocaleDateString(undefined, { month: 'long' })
  return `${name} ${year}`
}

/** Short day label, like "Nov 1". Display only. */
export function shortDayLabel(value: string): string {
  const [year, month, day] = value.split('-').map(Number)
  return new Date(year, month - 1, day).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

/** Short date range, like "Nov 1 – Nov 30". Display only. */
export function shortDateRange(startDate: string, endDate: string): string {
  return `${shortDayLabel(startDate)} – ${shortDayLabel(endDate)}`
}

export type PeriodSpan = { startDate: string; endDate: string; status: string }

/** True when an open span covers today (inclusive, YYYY-MM-DD). */
export function coversToday(span: PeriodSpan, today: string): boolean {
  return span.status === 'open' && span.startDate <= today && today <= span.endDate
}

export type SetupSnapshot = {
  companyName: string
  companyLabel: string
  baseCurrency: string
  hasLogo: boolean
  openPeriodLabel: string
  openPeriodDates: string
  ownerNames: string[]
}

export type SetupCheck = {
  id: string
  word: string
  state: 'ready' | 'missing' | 'skipped'
  detail: string
  step: SetupStepId | null
}

// Server GetReadiness checks the wizard covers with its own steps and
// better fix actions. Kept in step with TestLedgerReadinessNamesActions
// in internal/server/ledger_test.go; a renamed server check reappears
// below the wizard list instead of breaking anything.
const wizardCoveredChecks = new Set([
  'Company set up',
  'Base currency chosen',
  'Currencies available',
  'Today falls in an open period',
])

export type ServerCheck = { name: string; ok: boolean; detail: string }

/** Wizard checklist rows in step order. */
export function buildWizardChecks(snapshot: SetupSnapshot, skipped: ReadonlySet<SetupStepId>): SetupCheck[] {
  return [
    {
      id: 'company',
      word: 'Company set up',
      state: snapshot.companyName ? 'ready' : 'missing',
      detail: snapshot.companyName ? `${snapshot.companyLabel} (${snapshot.companyName})` : 'Save the company first.',
      step: 'company',
    },
    {
      id: 'logo',
      word: 'Logo set',
      state: snapshot.hasLogo ? 'ready' : skipped.has('logo') ? 'skipped' : 'missing',
      detail: snapshot.hasLogo ? 'The workspace mark shows in the header.' : 'Add it in Settings any time.',
      step: 'logo',
    },
    {
      id: 'currency',
      word: 'Base money chosen',
      state: snapshot.baseCurrency ? 'ready' : 'missing',
      detail: snapshot.baseCurrency ? snapshot.baseCurrency : 'Pick the money the company counts in.',
      step: 'currency',
    },
    {
      id: 'period',
      word: 'First period open',
      state: snapshot.openPeriodLabel ? 'ready' : 'missing',
      detail: snapshot.openPeriodLabel ? `${snapshot.openPeriodLabel} (${snapshot.openPeriodDates})` : 'Open the first working month.',
      step: 'period',
    },
    {
      id: 'invite',
      word: 'Owner invited',
      state: snapshot.ownerNames.length > 0 ? 'ready' : skipped.has('invite') ? 'skipped' : 'missing',
      detail:
        snapshot.ownerNames.length > 0
          ? snapshot.ownerNames.join(', ')
          : 'Invite one from Access any time.',
      step: 'invite',
    },
  ]
}

/** Server books checks the wizard does not cover itself. */
export function buildLedgerChecks(checks: ServerCheck[]): ServerCheck[] {
  return checks.filter((check) => !wizardCoveredChecks.has(check.name))
}

/** First step still missing, or null when the wizard list is complete. */
export function firstMissingStep(checks: SetupCheck[]): SetupStepId | null {
  for (const check of checks) {
    if (check.state === 'missing' && check.step) return check.step
  }
  return null
}
