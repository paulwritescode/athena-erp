import { describe, expect, it } from 'vitest'
import { pageCount, pageText, rangeText, rowNumber, selectionText } from './dataTableModel'

describe('table footer text', () => {
  it('shows the range with a total', () => {
    expect(rangeText({ pageRowCount: 25, page: 2, pageSize: 25, total: 214, hasNext: true })).toBe('Showing 26–50 of 214')
    expect(pageText({ pageRowCount: 25, page: 2, pageSize: 25, total: 214, hasNext: true })).toBe('Page 2 of 9')
  })

  it('shows a partial last page', () => {
    expect(rangeText({ pageRowCount: 14, page: 9, pageSize: 25, total: 214, hasNext: false })).toBe('Showing 201–214 of 214')
    expect(pageCount(214, 25)).toBe(9)
  })

  it('shows no total for keyset pages', () => {
    expect(rangeText({ pageRowCount: 25, page: 2, pageSize: 25, total: null, hasNext: true })).toBe('Showing 26–50')
    expect(pageText({ pageRowCount: 25, page: 2, pageSize: 25, total: null, hasNext: true })).toBe('Page 2')
    expect(pageCount(null, 25)).toBeNull()
  })

  it('shows zeros on an empty page', () => {
    expect(rangeText({ pageRowCount: 0, page: 1, pageSize: 25, total: 0, hasNext: false })).toBe('Showing 0–0 of 0')
  })

  it('numbers rows across pages', () => {
    expect(rowNumber(1, 25, 0)).toBe(1)
    expect(rowNumber(2, 25, 0)).toBe(26)
    expect(rowNumber(2, 25, 24)).toBe(50)
  })

  it('summarizes the selection', () => {
    expect(selectionText(3, 214)).toBe('3 of 214 selected')
    expect(selectionText(3, null)).toBe('3 selected')
  })
})
