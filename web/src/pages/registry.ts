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
import { BookOpen, ChartColumn, ClipboardCheck, ClipboardList, FileText, Folder, House, Inbox, LayoutGrid, LayoutTemplate, ListChecks, Package, ScrollText, Settings, Users } from '@lucide/vue'
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
  load: () => Promise<{ default: Component }>
  props?: (ctx: PageContext) => Record<string, unknown>
}

export const pageEntries: PageEntry[] = [
  {
    key: 'overview', title: 'Overview', shortTitle: 'Overview', blurb: 'Workspace health and where to go next.', icon: House,
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/OverviewPage.vue'),
    props: (ctx) => ({ recordTypes: ctx.recordTypes, databaseState: ctx.databaseState, apiState: ctx.apiState }),
  },
  {
    key: 'record-types', title: 'Record Types', shortTitle: 'Types', blurb: 'Define the tables of the workspace.', icon: LayoutTemplate,
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/RecordTypesPage.vue'),
    props: (ctx) => ({ recordTypes: ctx.recordTypes, currentRecordType: ctx.currentRecordType, databaseState: ctx.databaseState }),
  },
  {
    key: 'packs', title: 'Packs', shortTitle: 'Packs', blurb: 'Installable bundles of Record Types.', icon: Package,
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/PacksPage.vue'),
  },
  {
    key: 'rules', title: 'Rules', shortTitle: 'Rules', blurb: 'Configured checks and small automations.', icon: ScrollText,
    modes: ['build'], gate: { anyPermission: ['rules:read'] }, nav: true,
    load: () => import('@/pages/RulesPage.vue'),
    props: (ctx) => ({ canWrite: ctx.canWriteRules, recordTypes: ctx.recordTypes }),
  },
  {
    key: 'files', title: 'Files', shortTitle: 'Files', blurb: 'Documents attached to Records.', icon: Folder,
    modes: ['build'], gate: { anyPermission: ['files:read'] }, nav: true,
    load: () => import('@/pages/FilesPage.vue'),
    props: (ctx) => ({ canWrite: ctx.canWriteFiles }),
  },
  {
    key: 'access', title: 'Access', shortTitle: 'Access', blurb: 'People, roles and who may do what.', icon: Users,
    modes: ['build'], gate: { anyPermission: ['access:users', 'access:roles'] }, nav: true,
    load: () => import('@/pages/AccessPage.vue'),
    props: (ctx) => ({ permissions: ctx.permissions, username: ctx.username, recordTypes: ctx.recordTypes }),
  },
  {
    key: 'ledger', title: 'Ledger', shortTitle: 'Ledger', blurb: 'Companies, periods and accounts.', icon: BookOpen,
    modes: ['build'], gate: { anyPermission: ['ledger:read'] }, nav: true,
    load: () => import('@/pages/LedgerSetupPage.vue'),
    props: (ctx) => ({ canWrite: ctx.canWriteLedger }),
  },
  {
    key: 'reports', title: 'Reports', shortTitle: 'Reports', blurb: 'Lists and totals from your Records.', icon: ChartColumn,
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/ReportsPage.vue'),
  },
  {
    key: 'jobs', title: 'Jobs', shortTitle: 'Jobs', blurb: 'Imports, exports and scheduled work.', icon: ListChecks,
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/JobsPage.vue'),
  },
  {
    key: 'settings', title: 'Settings', shortTitle: 'Settings', blurb: 'Workspace preferences.', icon: Settings,
    modes: ['build'], gate: {}, nav: true,
    load: () => import('@/pages/SettingsPage.vue'),
  },
  {
    key: 'setup', title: 'Setup', shortTitle: 'Setup', blurb: 'Get the workspace ready.', icon: ClipboardList,
    modes: ['build'], gate: { build: true }, nav: true,
    load: () => import('@/pages/SetupPage.vue'),
  },
  {
    key: 'queue', title: 'Work queue', shortTitle: 'Queue', blurb: 'Your work across the workspace, in one list.', icon: Inbox,
    modes: ['view'], gate: {}, nav: true,
    load: () => import('@/pages/WorkQueuePage.vue'),
  },
  {
    key: 'count', title: 'Count', shortTitle: 'Count', blurb: 'Stock counts and quick tallies.', icon: ClipboardCheck,
    modes: ['view'], gate: {}, nav: true,
    load: () => import('@/pages/CountPage.vue'),
  },
  {
    key: 'gallery', title: 'Gallery', shortTitle: 'Gallery', blurb: 'Every shared composition in every state.', icon: LayoutGrid,
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
