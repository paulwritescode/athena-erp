// Canonical desk addresses, as pure functions so the suite pins them. Build
// pages live under /build/<key>; record lists under /view/<type>[/<record>];
// the View stubs under the reserved /view/-/<key> so a Record Type keyed
// `queue` or `count` keeps its own URL. Bare /view is the record list with
// whatever type the desk resolves.
import { buildPageKeys, viewStubKeys } from '@/pages/registry'
import type { Page } from '@/types'

export type ParsedPath =
  | { kind: 'build'; page: Page }
  | { kind: 'stub'; page: Page }
  | { kind: 'records'; recordType: string; record: string }
  | { kind: 'view' }

function decode(part: string): string {
  try {
    return decodeURIComponent(part)
  } catch {
    return part
  }
}

export function parseRoutePath(pathname: string): ParsedPath | null {
  const segments = pathname
    .split('/')
    .filter((part) => part.length > 0)
    .map(decode)
  if (segments.length === 0) return null
  if (segments[0] === 'build' && (buildPageKeys() as string[]).includes(segments[1] ?? '')) {
    return { kind: 'build', page: segments[1] as Page }
  }
  if (segments[0] === 'view' && segments[1] === '-' && (viewStubKeys() as string[]).includes(segments[2] ?? '')) {
    return { kind: 'stub', page: segments[2] as Page }
  }
  if (segments[0] === 'view' && segments[1] && segments[1] !== '-') {
    return { kind: 'records', recordType: segments[1], record: segments[2] ?? '' }
  }
  if (segments[0] === 'view') return { kind: 'view' }
  return null
}

export function buildRoutePath(mode: 'build' | 'view', page: Page, selectedRecordType: string, recordRoute: string): string {
  if (mode === 'build') return `/build/${page}`
  if (page === 'queue' || page === 'count') return `/view/-/${page}`
  if (selectedRecordType) {
    const base = `/view/${encodeURIComponent(selectedRecordType)}`
    return recordRoute ? `${base}/${encodeURIComponent(recordRoute)}` : base
  }
  return '/view'
}
