// The page registry drives desk navigation (sidebar, bottom nav, command
// palette) and the page switch from one list. Each entry names its icon,
// which mode it belongs to, who may see it, and how to load it; pages load
// lazily so the first paint stays small.
//
// Gates mirror the pre-B2 desk: Overview, Record Types and Packs need the
// build permission; Rules, Files, Access and Ledger need their read
// permission; the View stubs are open to every signed-in user. Anything
// hidden here is also checked on the server.
import type { Component } from 'vue'
import { icon } from '@/icons'
import { FileText } from '@/icons'
import type { Page, RecordType } from '@/types'

// The data a registry page may need, assembled by the page switch from desk
// state. Entries pick what they need through props().
export type PageContext = {
  recordTypes: RecordType[]
  currentRecordType: RecordType
  databaseState: string
  apiState: string
  permissions: string[]
  username: string
  canWriteRules: boolean
  canWriteFiles: boolean
  canWriteLedger: boolean
  typesLoaded: boolean
  typesLoadFailed: boolean
}

export type PageEntry = {
  key: Page
  title: string
  // The bottom-nav label: short enough that it never truncates.
  shortTitle: string
  blurb: string
  icon: Component
  modes: Array<'build' | 'view'>
  // build needs the build permission; anyPermission needs any one of the
  // listed Grants; neither means every signed-in user.
  gate: { build?: boolean; anyPermission?: string[] }
  // Gallery-style pages load by address only and stay out of the nav.
  nav: boolean
  // The Build sidebar group (§15); undefined renders without a heading.
  group?: string
  load: () => Promise<{ default: Component }>
  props?: (ctx: PageContext) => Record<string, unknown>
}

export const pageEntries: PageEntry[] = [
  {
    key: 'overview', title: 'Overview', shortTitle: 'Home', blurb: 'Workspace health and where to go next.', icon: icon('house'),
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/OverviewPage.vue'),
    props: (ctx) => ({ recordTypes: ctx.recordTypes, databaseState: ctx.databaseState, apiState: ctx.apiState }),
  },
  {
    key: 'record-types', title: 'Record Types', shortTitle: 'Types', blurb: 'Define the kinds of records you keep.', icon: icon('layout-template'),
    modes: ['build'], gate: { build: true }, nav: true, group: 'build',
    load: () => import('@/pages/RecordTypesPage.vue'),
    props: (ctx) => ({ recordTypes: ctx.recordTypes, currentRecordType: ctx.currentRecordType, databaseState: ctx.databaseState, typesLoaded: ctx.typesLoaded, typesLoadFailed: ctx.typesLoadFailed, username: ctx.username }),
  },
  {
    key: 'packs', title: 'Packs', shortTitle: 'Packs', blurb: 'Installable bundles of Record Types.', icon: icon('package'),
    modes: ['build'], gate: { build: true }, nav: true, group: 'build',
    load: () => import('@/pages/PacksPage.vue'),
  },
  {
    key: 'rules', title: 'Rules', shortTitle: 'Rules', blurb: 'Configured checks and small automations.', icon: icon('scroll-text'),
    modes: ['build'], gate: { anyPermission: ['rules:read'] }, nav: true, group: 'build',
    load: () => import('@/pages/RulesPage.vue'),
    props: (ctx) => ({ canWrite: ctx.canWriteRules, recordTypes: ctx.recordTypes, username: ctx.username }),
  },
  {
    key: 'files', title: 'Files', shortTitle: 'Files', blurb: 'Documents attached to Records.', icon: icon('folder'),
    modes: ['build'], gate: { anyPermission: ['files:read'] }, nav: true, group: 'build',
    load: () => import('@/pages/FilesPage.vue'),
    props: (ctx) => ({ canWrite: ctx.canWriteFiles, username: ctx.username }),
  },
  {
    key: 'access', title: 'Access', shortTitle: 'Access', blurb: 'People, roles and who may do what.', icon: icon('users'),
    modes: ['build'], gate: { anyPermission: ['access:users', 'access:roles'] }, nav: true, group: 'administer',
    load: () => import('@/pages/AccessPage.vue'),
    props: (ctx) => ({ permissions: ctx.permissions, username: ctx.username, recordTypes: ctx.recordTypes }),
  },
  {
    key: 'ledger', title: 'Ledger', shortTitle: 'Ledger', blurb: 'Companies, periods and accounts.', icon: icon('book-open'),
    modes: ['build'], gate: { anyPermission: ['ledger:read'] }, nav: true, group: 'operate',
    load: () => import('@/pages/LedgerSetupPage.vue'),
    props: (ctx) => ({ canWrite: ctx.canWriteLedger, username: ctx.username }),
  },
  {
    key: 'reports', title: 'Reports', shortTitle: 'Reports', blurb: 'Lists and totals from your Records.', icon: icon('chart-column'),
    modes: ['build'], gate: { build: true }, nav: true, group: 'operate',
    load: () => import('@/pages/ReportsPage.vue'),
  },
  {
    key: 'jobs', title: 'Jobs', shortTitle: 'Jobs', blurb: 'Imports, exports and scheduled work.', icon: icon('list-checks'),
    modes: ['build'], gate: { build: true }, nav: true, group: 'operate',
    load: () => import('@/pages/JobsPage.vue'),
  },
  {
    key: 'settings', title: 'Settings', shortTitle: 'Settings', blurb: 'Workspace preferences.', icon: icon('settings'),
    modes: ['build'], gate: {}, nav: true, group: 'administer',
    load: () => import('@/pages/SettingsPage.vue'),
    props: (ctx) => ({ permissions: ctx.permissions }),
  },
  {
    key: 'setup', title: 'Setup', shortTitle: 'Setup', blurb: 'Get the workspace ready.', icon: icon('clipboard-list'),
    modes: ['build'], gate: { build: true }, nav: true, group: 'administer',
    load: () => import('@/pages/SetupPage.vue'),
  },
  {
    key: 'queue', title: 'Work queue', shortTitle: 'My work', blurb: 'Your work across the workspace, in one list.', icon: icon('inbox'),
    modes: ['view'], gate: {}, nav: true,
    load: () => import('@/pages/WorkQueuePage.vue'),
  },
  {
    key: 'count', title: 'Count', shortTitle: 'Count', blurb: 'Stock counts and quick tallies.', icon: icon('clipboard-check'),
    modes: ['view'], gate: {}, nav: true,
    load: () => import('@/pages/CountPage.vue'),
  },
  {
    key: 'gallery', title: 'Gallery', shortTitle: 'Gallery', blurb: 'Every shared composition in every state.', icon: icon('layout-grid'),
    modes: ['build'], gate: { build: true }, nav: false,
    load: () => import('@/pages/GalleryPage.vue'),
  },
]

export function pageEntry(key: Page): PageEntry | undefined {
  return pageEntries.find((entry) => entry.key === key)
}

// Every static page key, for route validation. View record lists address
// Record Types by key and need no entry.
export function pageKeys(): Page[] {
  return pageEntries.map((entry) => entry.key)
}

export function buildPageKeys(): Page[] {
  return pageEntries.filter((entry) => entry.modes.includes('build')).map((entry) => entry.key)
}

export function viewStubKeys(): Page[] {
  return pageEntries.filter((entry) => entry.modes.includes('view')).map((entry) => entry.key)
}

export function isEntryVisible(entry: PageEntry, canBuild: boolean, permissions: string[], authenticated = true): boolean {
  // Gates describe signed-in users: nothing is visible signed out. The desk
  // only renders signed in, so callers there keep the default; the landing
  // logic passes the real state so a signed-out visitor matches nothing,
  // exactly like the pre-B2 nav did.
  if (!authenticated) return false
  if (entry.gate.build && !canBuild) return false
  const any = entry.gate.anyPermission
  if (any && !any.some((permission) => permissions.includes(permission))) return false
  return true
}

export function visibleBuildEntries(canBuild: boolean, permissions: string[], authenticated = true): PageEntry[] {
  return pageEntries.filter((entry) => entry.modes.includes('build') && entry.nav && isEntryVisible(entry, canBuild, permissions, authenticated))
}

export function visibleBuildKeys(canBuild: boolean, permissions: string[], authenticated = true): Page[] {
  return visibleBuildEntries(canBuild, permissions, authenticated).map((entry) => entry.key)
}

export function visibleViewStubs(): PageEntry[] {
  return pageEntries.filter((entry) => entry.modes.includes('view') && entry.nav)
}

// Record Types are View nav items built from data, not registry entries; they
// share the fallback icon until B3 gives the registry per-type icons.
export { FileText as recordTypeIcon }

// Build sidebar groups in §15 order. Ungrouped entries (Overview) render
// first without a heading; a group with no visible entry is skipped.
export const buildGroupOrder: Array<{ key: string; label: string }> = [
  { key: 'build', label: 'Build' },
  { key: 'operate', label: 'Operate' },
  { key: 'administer', label: 'Administer' },
]

export type BuildGroup = { key: string | null; label: string | null; entries: PageEntry[] }

export function visibleBuildGroups(canBuild: boolean, permissions: string[], authenticated = true): BuildGroup[] {
  const visible = visibleBuildEntries(canBuild, permissions, authenticated)
  const groups: BuildGroup[] = []
  const ungrouped = visible.filter((entry) => !entry.group)
  if (ungrouped.length > 0) groups.push({ key: null, label: null, entries: ungrouped })
  for (const { key, label } of buildGroupOrder) {
    const entries = visible.filter((entry) => entry.group === key)
    if (entries.length > 0) groups.push({ key, label, entries })
  }
  return groups
}

// View Record Types grouped by the Pack or Module that installed them;
// ungrouped ones sit under "Records" (§15). Packs are not implemented, so
// every type is ungrouped today and this returns one group.
export type RecordTypeGroup<T extends { name: string; label: string; pack?: string | null; module?: string | null }> = {
  key: string
  label: string
  types: T[]
}

export function recordTypeGroups<T extends { name: string; label: string; pack?: string | null; module?: string | null }>(
  types: T[],
): Array<RecordTypeGroup<T>> {
  const groups = new Map<string, RecordTypeGroup<T>>()
  for (const type of types) {
    const owner = type.pack || type.module
    const key = owner ? `pack:${owner}` : 'records'
    const label = owner ?? 'Records'
    const group = groups.get(key)
    if (group) group.types.push(type)
    else groups.set(key, { key, label, types: [type] })
  }
  return [...groups.values()]
}

// Collapsed sidebar groups, remembered per user in localStorage. The storage
// is injectable so the logic is unit-testable without a DOM.
export function navGroupsKey(username: string): string {
  return `athena-nav-groups:${username}`
}

export function loadCollapsedGroups(username: string, storage?: Pick<Storage, 'getItem'> | undefined): Set<string> {
  const store = storage ?? (typeof globalThis.localStorage === 'undefined' ? undefined : globalThis.localStorage)
  if (!username || !store) return new Set()
  try {
    const raw = store.getItem(navGroupsKey(username))
    if (!raw) return new Set()
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed.filter((key): key is string => typeof key === 'string'))
  } catch {
    return new Set()
  }
}

export function saveCollapsedGroups(
  username: string,
  collapsed: Set<string> | string[],
  storage?: Pick<Storage, 'setItem' | 'removeItem'> | undefined,
): void {
  const store = storage ?? (typeof globalThis.localStorage === 'undefined' ? undefined : globalThis.localStorage)
  if (!username || !store) return
  const keys = [...collapsed]
  try {
    if (keys.length === 0) store.removeItem(navGroupsKey(username))
    else store.setItem(navGroupsKey(username), JSON.stringify(keys))
  } catch {
    // Private browsing and quotas must never break navigation.
  }
}
