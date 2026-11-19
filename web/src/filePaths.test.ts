import { describe, expect, it } from 'vitest'
import cases from '../../internal/files/testdata/paths.json'
import { validateFilePath, withinFileLimit } from './filePaths'

// The server test reads the same table (internal/files/files_test.go).
describe('validateFilePath', () => {
  it.each(cases.valid)('accepts %s', (path) => {
    expect(validateFilePath(path)).toBeNull()
  })
  it.each(Object.entries(cases.invalid))('rejects %j (%s)', (path) => {
    expect(validateFilePath(path)).not.toBeNull()
  })
})

describe('withinFileLimit', () => {
  it('counts bytes, not characters', () => {
    expect(withinFileLimit('a'.repeat(1 << 20))).toBe(true)
    expect(withinFileLimit('a'.repeat((1 << 20) + 1))).toBe(false)
    expect(withinFileLimit('é'.repeat((1 << 19) + 1))).toBe(false)
  })
})
