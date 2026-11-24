// Pure footer and paging math for DataTable, kept here so the suite pins
// the exact words without mounting the component.
export type RangeInput = {
  // Rows on this page (after filtering, before slicing in client mode the
  // caller passes the page slice length via page rows; in server mode the
  // number returned).
  pageRowCount: number
  // 1-based page. Server keyset pages count from the tokens walked so far.
  page: number
  pageSize: number
  // Total rows, when the API provides one. Null means unknown (keyset).
  total: number | null
  hasNext: boolean
}

export function pageStartRow(input: RangeInput): number {
  if (input.pageRowCount === 0) return 0
  return (input.page - 1) * input.pageSize + 1
}

export function pageEndRow(input: RangeInput): number {
  if (input.pageRowCount === 0) return 0
  return (input.page - 1) * input.pageSize + input.pageRowCount
}

// "Showing 26–50 of 214", or "Showing 26–50" when the total is unknown.
export function rangeText(input: RangeInput): string {
  const start = pageStartRow(input)
  const end = pageEndRow(input)
  if (input.total === null) return `Showing ${start}–${end}`
  return `Showing ${start}–${end} of ${input.total}`
}

// "Page 2 of 9", or "Page 2" when the total is unknown.
export function pageText(input: RangeInput): string {
  if (input.total === null) return `Page ${input.page}`
  const pages = Math.max(1, Math.ceil(input.total / input.pageSize))
  return `Page ${input.page} of ${pages}`
}

export function pageCount(total: number | null, pageSize: number): number | null {
  if (total === null) return null
  return Math.max(1, Math.ceil(total / pageSize))
}

// The row number of the index-th row on the page (0-based index),
// continuing across pages: page 2 of 25 starts at 26.
export function rowNumber(page: number, pageSize: number, index: number): number {
  return (page - 1) * pageSize + index + 1
}

// Selection summary for the footer's left side: "3 of 214 selected" with a
// total, otherwise "3 selected".
export function selectionText(selected: number, total: number | null): string {
  if (total === null) return `${selected} selected`
  return `${selected} of ${total} selected`
}

// The footer's left side: tables with selection always show the count ("0
// of 5 selected" at rest, §21); tables without it show the range.
export function footerLeftText(hasBulk: boolean, selected: number, range: RangeInput): string {
  if (hasBulk) return selectionText(selected, range.total)
  return rangeText(range)
}
