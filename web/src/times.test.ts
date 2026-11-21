import { describe, expect, it } from 'vitest'
import { formatWhen, formatWhenFull } from './times'

function iso(year: number, month: number, day: number, hour = 13, minute = 36): string {
  return new Date(year, month - 1, day, hour, minute).toISOString()
}

describe('formatWhen', () => {
  it('reads today with the time', () => {
    const now = new Date(2026, 10, 21, 18, 5)
    expect(formatWhen(new Date(2026, 10, 21, 13, 36).toISOString(), now)).toMatch(/^Today \d{1,2}:\d{2}/)
  })

  it('reads yesterday bare', () => {
    const now = new Date(2026, 10, 21, 18, 5)
    expect(formatWhen(iso(2026, 11, 20), now)).toBe('Yesterday')
  })

  it('reads older and future dates as day month year', () => {
    const now = new Date(2026, 10, 21, 18, 5)
    const past = formatWhen(iso(2026, 10, 5), now)
    expect(past).toMatch(/^5 \S+ 2026$/)
    const future = formatWhen(iso(2026, 11, 28), now)
    expect(future).toMatch(/^28 \S+ 2026$/)
  })

  it('returns empty for empty or invalid input', () => {
    expect(formatWhen('')).toBe('')
    expect(formatWhen('not-a-date')).toBe('')
    expect(formatWhenFull('')).toBe('')
  })
})

describe('formatWhenFull', () => {
  it('carries seconds', () => {
    const full = formatWhenFull(iso(2026, 11, 21))
    expect(full).toMatch(/\d{1,2}:\d{2}:\d{2}/)
  })
})
