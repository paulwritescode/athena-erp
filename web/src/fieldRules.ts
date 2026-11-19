import type { Field } from './types'

// Mirrors the server's checks in internal/server/records.go (fieldArgument and
// bindValues) and internal/server/kinds.go so people see the same error before
// the request is sent. The server still validates every write. Decimals are
// compared as BigInt so no value passes through a binary fraction.
const maxLength: Partial<Record<string, number>> = { Text: 255, Email: 320, Phone: 40, Select: 120, Enum: 120, 'Long text': 65535 }
const decimal = /^-?[0-9]{1,12}(\.[0-9]{1,6})?$/
const amount = /^-?[0-9]{1,40}(\.[0-9]{1,40})?$/
const isoDate = /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const maxQuantityScale = 6
const int64Max = 2n ** 63n - 1n
const int64Min = -(2n ** 63n)

// Date.parse accepts 2026-02-30 and rolls it over, so compare the round trip.
function isCalendarDate(value: string): boolean {
  if (!isoDate.test(value)) return false
  const [year = 0, month = 0, day = 0] = value.split('-').map(Number)
  const date = new Date(Date.UTC(year, month - 1, day))
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
}

/** Document states, as the kernel numbers them (internal/lifecycle). */
export const docstates = { draft: 0, submitted: 1, cancelled: 2 } as const

/**
 * Mirrors the server's immutability rule (internal/server/records.go,
 * UpdateRecord): a draft is editable, a submitted document changes only in
 * fields marked allow_on_submit, and a cancelled document does not change.
 * A Record Type that is not submittable is always editable.
 */
export function fieldEditable(field: Field, submittable: boolean, docstate: number): boolean {
  if (!submittable || docstate === docstates.draft) return true
  return docstate === docstates.submitted && field.allow_on_submit === true
}

/** Splits a decimal string into its sign, whole digits and fraction digits. */
function parts(text: string): { negative: boolean; whole: string; fraction: string } {
  const negative = text.startsWith('-')
  const [whole = '', fraction = ''] = (negative ? text.slice(1) : text).split('.')
  return { negative, whole, fraction }
}

/** Compares two valid decimal strings exactly: negative, zero or positive. */
export function compareDecimal(a: string, b: string): number {
  const x = parts(a)
  const y = parts(b)
  const scale = Math.max(x.fraction.length, y.fraction.length)
  const scaled = (p: ReturnType<typeof parts>) => (p.negative ? -1n : 1n) * BigInt(p.whole + p.fraction.padEnd(scale, '0'))
  const diff = scaled(x) - scaled(y)
  return diff < 0n ? -1 : diff > 0n ? 1 : 0
}

/** The amount in minor units, or null when it has more places than scale or does not fit 64 bits. */
function minorUnits(value: string, scale: number): bigint | null {
  const { negative, whole, fraction } = parts(value)
  if (fraction.length > scale) return null
  const minor = (negative ? -1n : 1n) * BigInt(whole + fraction.padEnd(scale, '0'))
  return minor < int64Min || minor > int64Max ? null : minor
}

function compare(field: Field, a: string, b: string): number {
  return field.type === 'Date' ? (a < b ? -1 : a > b ? 1 : 0) : compareDecimal(a, b)
}

function checkRange(field: Field, value: string): string {
  if (field.min && compare(field, value, field.min) < 0) return `Enter ${field.min} or more.`
  if (field.max && compare(field, value, field.max) > 0) return `Enter ${field.max} or less.`
  return ''
}

/** Returns an error message for one field value, or an empty string when it is valid. */
export function validateField(field: Field, text: string): string {
  if (field.type === 'Checkbox') return ''
  const value = text.trim()
  if (!value) return field.required ? `${field.label} is required.` : ''
  const typeLimit = maxLength[field.type]
  const limit = typeLimit !== undefined && field.max_length ? Math.min(typeLimit, field.max_length) : typeLimit
  if (limit !== undefined && [...value].length > limit) return `Use at most ${limit} characters.`
  if (field.pattern) {
    try {
      if (!new RegExp(`^(?:${field.pattern})$`, 'u').test(value)) return 'Use the required format.'
    } catch {
      // The server uses RE2 and has the final say on a pattern JavaScript cannot read.
    }
  }
  switch (field.type) {
    case 'Email':
      return email.test(value) ? '' : 'Enter an email address, like name@example.com.'
    case 'Number':
    case 'Currency':
      return decimal.test(value) ? checkRange(field, value) : 'Enter a number with up to 12 digits before and 6 after the decimal point.'
    case 'Quantity': {
      if (!decimal.test(value)) return 'Enter a number with up to 12 digits before and 6 after the decimal point.'
      const scale = field.scale ?? maxQuantityScale
      if (parts(value).fraction.length > scale) return `Use at most ${scale} decimal places.`
      return checkRange(field, value)
    }
    case 'Money': {
      const scale = field.scale ?? 0
      if (!amount.test(value)) return `Enter an amount in ${field.currency ?? 'the currency'}, like 12.${'0'.repeat(Math.max(scale, 1))}.`
      if (minorUnits(value, scale) === null) return `Use at most ${scale} decimal places, and a smaller amount.`
      return checkRange(field, value)
    }
    case 'Date':
      return isCalendarDate(value) ? checkRange(field, value) : 'Enter a date.'
    case 'Enum':
      return field.values?.some((choice) => choice.value === value) ? '' : 'Choose one of the listed values.'
    case 'Link':
      return uuid.test(value) ? '' : `Choose a ${field.target_record_type ?? 'linked'} record.`
    default:
      return ''
  }
}

// Naming (internal/numbering and internal/server/naming.go). A manual document
// name has at most 140 characters and cannot look like a Record ID; the
// server also refuses a name another document already holds.
export function validateManualName(text: string): string {
  const value = text.trim()
  if (!value) return ''
  if ([...value].length > 140) return 'Use at most 140 characters.'
  if (uuid.test(value)) return 'A name cannot look like a Record ID.'
  return ''
}

/**
 * Checks a naming series such as SINV-{yyyy}-{#####}: literal text, the
 * tokens {yyyy}, {yy} and {mm}, and exactly one counter token of 1 to 9 "#".
 * Returns an error message, or an empty string when the pattern is valid.
 */
export function validateNamingSeries(pattern: string): string {
  const value = pattern.trim()
  if (!value) return ''
  if (value.length > 100) return 'Use at most 100 characters.'
  let counters = 0
  let rest = value
  while (rest) {
    const at = rest.search(/[{}]/)
    if (at < 0) break
    if (rest[at] === '}') return 'The pattern has an unmatched "}".'
    const end = rest.indexOf('}', at)
    if (end < 0) return 'The pattern has an unmatched "{".'
    const token = rest.slice(at + 1, end)
    if (/^#{1,9}$/.test(token)) counters++
    else if (!['yyyy', 'yy', 'mm'].includes(token)) return `{${token}} is not a series token; use {yyyy}, {yy}, {mm} or {#####}.`
    rest = rest.slice(end + 1)
  }
  return counters === 1 ? '' : 'The pattern needs exactly one counter token such as {#####}.'
}
