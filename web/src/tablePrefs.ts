// Per-user table preferences: visible columns and rows per page, in
// localStorage. Keys carry the table id and the username so one browser
// remembers each person's layout separately; without a username the layout
// is shared. Storage is injectable so the logic is unit-testable.
export const PAGE_SIZE_OPTIONS = [10, 25, 50, 100] as const
export const DEFAULT_PAGE_SIZE = 25

export function tableColumnsKey(tableId: string, username: string): string {
  return `athena-table-columns:${username || 'shared'}:${tableId}`
}

export function tablePageSizeKey(tableId: string, username: string): string {
  return `athena-table-page-size:${username || 'shared'}:${tableId}`
}

type Reader = Pick<Storage, 'getItem'>
type Writer = Pick<Storage, 'setItem' | 'removeItem'>

function store(): Storage | undefined {
  return typeof globalThis.localStorage === 'undefined' ? undefined : globalThis.localStorage
}

// Hidden column ids. Unknown ids are kept: a column hidden while it existed
// stays hidden if it comes back.
export function loadHiddenColumns(tableId: string, username: string, storage?: Reader | undefined): Set<string> {
  const backend = storage ?? store()
  if (!tableId || !backend) return new Set()
  try {
    const raw = backend.getItem(tableColumnsKey(tableId, username))
    if (!raw) return new Set()
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed.filter((id): id is string => typeof id === 'string'))
  } catch {
    return new Set()
  }
}

export function saveHiddenColumns(tableId: string, username: string, hidden: Set<string> | string[], storage?: Writer | undefined): void {
  const backend = storage ?? store()
  if (!tableId || !backend) return
  try {
    const ids = [...hidden]
    if (ids.length === 0) backend.removeItem(tableColumnsKey(tableId, username))
    else backend.setItem(tableColumnsKey(tableId, username), JSON.stringify(ids))
  } catch {
    // Private browsing and quotas must never break the table.
  }
}

export function loadPageSize(tableId: string, username: string, storage?: Reader | undefined): number {
  const backend = storage ?? store()
  if (!tableId || !backend) return DEFAULT_PAGE_SIZE
  try {
    const raw = backend.getItem(tablePageSizeKey(tableId, username))
    const size = raw === null ? NaN : Number.parseInt(raw, 10)
    return (PAGE_SIZE_OPTIONS as readonly number[]).includes(size) ? size : DEFAULT_PAGE_SIZE
  } catch {
    return DEFAULT_PAGE_SIZE
  }
}

export function savePageSize(tableId: string, username: string, size: number, storage?: Writer | undefined): void {
  const backend = storage ?? store()
  if (!tableId || !backend) return
  try {
    backend.setItem(tablePageSizeKey(tableId, username), String(size))
  } catch {
    // Private browsing and quotas must never break the table.
  }
}
