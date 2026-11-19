import type { Field } from './types'

// Mirrors the server's checks in internal/server/records.go (fieldArgument and
// bindValues) so people see the same error before the request is sent. The
// server still validates every write.
const maxLength: Partial<Record<string, number>> = { Text: 255, Email: 320, Phone: 40, Select: 120, 'Long text': 65535 }
const decimal = /^-?[0-9]{1,12}(\.[0-9]{1,6})?$/
const isoDate = /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

// Date.parse accepts 2026-02-30 and rolls it over, so compare the round trip.
function isCalendarDate(value: string): boolean {
  if (!isoDate.test(value)) return false
  const [year = 0, month = 0, day = 0] = value.split('-').map(Number)
  const date = new Date(Date.UTC(year, month - 1, day))
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
}

/** Returns an error message for one field value, or an empty string when it is valid. */
export function validateField(field: Field, text: string): string {
  if (field.type === 'Checkbox') return ''
  const value = text.trim()
  if (!value) return field.required ? `${field.label} is required.` : ''
  const limit = maxLength[field.type]
  if (limit !== undefined && [...value].length > limit) return `Use at most ${limit} characters.`
  switch (field.type) {
    case 'Email':
      return email.test(value) ? '' : 'Enter an email address, like name@example.com.'
    case 'Number':
    case 'Currency':
      return decimal.test(value) ? '' : 'Enter a number with up to 12 digits before and 6 after the decimal point.'
    case 'Date':
      return isCalendarDate(value) ? '' : 'Enter a date.'
    case 'Link':
      return uuid.test(value) ? '' : `Enter the ID of a ${field.target_record_type ?? 'linked'} record.`
    default:
      return ''
  }
}
