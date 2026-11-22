// The desk route lives in the address bar so a refresh or the back button
// returns to the same screen. The canonical form is the path (see
// routePaths.ts): /build/<page>, /view/<record-type>[/<record>] and the
// reserved /view/-/<stub>. The older query form still reads, so shared links
// keep working; it is never written. Each mode remembers its own last page.
// An address nothing parses is the not-found page; the address stays put so
// the person can see what failed. landedExplicit tells startup whether the
// address names a destination: only a bare address redirects, deep links
// keep their page and mode.
import { onUnmounted, ref, watch } from 'vue'
import { pageKeys } from '@/pages/registry'
import { buildRoutePath, parseRoutePath } from '@/routePaths'
import type { Page } from '@/types'

export type DeskRoute = ReturnType<typeof useDeskRoute>

function belongsTo(page: Page, mode: 'build' | 'view'): boolean {
  // In View, overview is the record-list sentinel; queue and count are
  // View-only. In Build every other key belongs.
  if (mode === 'view') return page === 'overview' || page === 'queue' || page === 'count'
  return page !== 'queue' && page !== 'count'
}

export function useDeskRoute() {
  // Resolves the default View list (first saved type). The composer owns the
  // library, so it installs this; flips resolve the final address first and
  // push exactly once.
  let defaultTypeResolver: () => string = () => ''
  const mode = ref<'build' | 'view'>('build')
  const page = ref<Page>('overview')
  const selectedRecordType = ref('')
  // The open Record in View: '' for the table, 'new' for the create form, or a Record ID.
  const recordRoute = ref('')
  // The open Record Type editor in Build: '' for the list, 'new' for the
  // create dialog over the list, or the type key.
  const recordTypeRoute = ref('')
  const lastBuildPage = ref<Page>('overview')
  const lastViewPage = ref<Page>('overview')
  // True while the address names no page. Any navigation clears it.
  const notFound = ref(false)
  const landedExplicit = window.location.pathname !== '/' || window.location.search !== ''
  // Next address write replaces instead of pushing. Resolution syncs use it
  // so Back never lands on an intermediate address.
  let replaceNext = false

  function syncLast() {
    if (mode.value === 'build' && belongsTo(page.value, 'build')) lastBuildPage.value = page.value
    if (mode.value === 'view' && belongsTo(page.value, 'view')) lastViewPage.value = page.value
  }

  function readLocation() {
    const parsed = parseRoutePath(window.location.pathname)
    const query = new URLSearchParams(window.location.search)
    // Every read recomputes: only an address nothing parses sets it again.
    notFound.value = false
    if (parsed?.kind === 'build') {
      mode.value = 'build'
      page.value = parsed.page
      recordRoute.value = ''
      recordTypeRoute.value = parsed.page === 'record-types' ? parsed.recordType ?? '' : ''
      // The editor address names its own selection; the list keeps the legacy
      // query fallback. A fresh /new starts unselected; the dialog names the
      // new type itself.
      if (recordTypeRoute.value === 'new') selectedRecordType.value = ''
      else if (recordTypeRoute.value) selectedRecordType.value = recordTypeRoute.value
      else selectedRecordType.value = query.get('record_type') ?? ''
    } else if (parsed?.kind === 'stub') {
      mode.value = 'view'
      page.value = parsed.page
      recordRoute.value = ''
      recordTypeRoute.value = ''
    } else if (parsed?.kind === 'records') {
      mode.value = 'view'
      page.value = 'overview'
      selectedRecordType.value = parsed.recordType
      recordRoute.value = parsed.record
      recordTypeRoute.value = ''
    } else if (parsed?.kind === 'view') {
      mode.value = 'view'
      page.value = 'overview'
      recordRoute.value = ''
      recordTypeRoute.value = ''
    } else {
      // A bare address (or the bare /build section root) reads the legacy
      // query form; any other unparsed address is the not-found page, with
      // mode and page left alone so the failed address stays in the bar.
      const segments = window.location.pathname.split('/').filter((part) => part.length > 0)
      const bare = segments.length === 0 || (segments.length === 1 && segments[0] === 'build')
      if (!bare) {
        notFound.value = true
        return
      }
      mode.value = query.get('mode') === 'view' ? 'view' : 'build'
      const requested = query.get('page')
      page.value = pageKeys().find((item) => item === requested) ?? 'overview'
      selectedRecordType.value = query.get('record_type') ?? ''
      recordRoute.value = mode.value === 'view' ? query.get('record') ?? '' : ''
      recordTypeRoute.value = ''
    }
    syncLast()
  }

  function targetLocation(): string {
    const path = buildRoutePath(mode.value, page.value, selectedRecordType.value, recordRoute.value, recordTypeRoute.value)
    const query = new URLSearchParams()
    // The editor address already names its type; other Build pages keep the
    // legacy selection query.
    if (mode.value === 'build' && selectedRecordType.value && !(page.value === 'record-types' && recordTypeRoute.value)) query.set('record_type', selectedRecordType.value)
    return query.toString().length > 0 ? `${path}?${query.toString()}` : path
  }

  function writeLocation() {
    const target = targetLocation()
    const current = `${window.location.pathname}${window.location.search}`
    if (target !== current) window.history.pushState(null, '', target)
  }

  function replaceLocation() {
    const target = targetLocation()
    const current = `${window.location.pathname}${window.location.search}`
    if (target !== current) window.history.replaceState(null, '', target)
  }

  readLocation()
  window.addEventListener('popstate', readLocation)
  onUnmounted(() => window.removeEventListener('popstate', readLocation))

  watch([mode, page, selectedRecordType, recordRoute, recordTypeRoute], () => {
    if (replaceNext) {
      replaceNext = false
      replaceLocation()
    } else {
      writeLocation()
    }
  })
  watch(page, syncLast)
  // Normalize once on load (legacy query URLs become paths) without adding
  // a history entry. A failed address is never normalized away.
  if (!notFound.value) replaceLocation()

  // The one way to flip modes: page and mode change together, so the flip is
  // a single history entry restoring that mode's own last page.
  function setMode(next: 'build' | 'view') {
    notFound.value = false
    if (next === 'build') {
      recordRoute.value = ''
      page.value = lastBuildPage.value
    } else {
      page.value = lastViewPage.value
      recordTypeRoute.value = ''
      if (page.value === 'overview' && !selectedRecordType.value) {
        const first = defaultTypeResolver()
        if (first) selectedRecordType.value = first
      }
    }
    mode.value = next
  }

  function openBuildPage(key: Page) {
    notFound.value = false
    lastBuildPage.value = key
    mode.value = 'build'
    page.value = key
    recordTypeRoute.value = ''
  }

  function openRecordType(name: string) {
    notFound.value = false
    mode.value = 'view'
    page.value = 'overview'
    lastViewPage.value = 'overview'
    selectedRecordType.value = name
    recordRoute.value = ''
    recordTypeRoute.value = ''
  }

  function openViewStub(key: 'queue' | 'count') {
    notFound.value = false
    lastViewPage.value = key
    mode.value = 'view'
    page.value = key
    recordRoute.value = ''
    recordTypeRoute.value = ''
  }

  // Opens a Record Type in the editor: a saved key (which becomes the
  // selection), or 'new' for the create dialog over the list. The dialog
  // renders from the route alone, so a reload reopens it as-is.
  function openRecordTypeEditor(key: string) {
    notFound.value = false
    lastBuildPage.value = 'record-types'
    mode.value = 'build'
    page.value = 'record-types'
    if (key !== 'new') selectedRecordType.value = key
    recordTypeRoute.value = key
  }

  // After the dialog creates its type, the address becomes the new key
  // without adding history: going back to /new would only reopen it.
  function replaceRecordTypeEditor(key: string) {
    notFound.value = false
    replaceNext = true
    recordTypeRoute.value = key
  }

  // Names the resolved list without adding history: the bare address it
  // replaces was never a page the user visited.
  function resolveSelected(name: string) {
    if (selectedRecordType.value) return
    notFound.value = false
    replaceNext = true
    selectedRecordType.value = name
  }

  return {
    mode, page, selectedRecordType, recordRoute, recordTypeRoute, landedExplicit, notFound, setMode, openBuildPage, openRecordType, openViewStub, openRecordTypeEditor, replaceRecordTypeEditor, resolveSelected,
    setDefaultTypeResolver(resolver: () => string) {
      defaultTypeResolver = resolver
    },
  }
}
