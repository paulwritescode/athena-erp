<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { classify, describeFailure, type Failure } from './apiErrors'
import { authClient, deleteRecordType, listRecordTypes, onSessionEnded, planRecordTypeSave, saveRecordType, setCsrfToken, systemClient, type SaveAnswers, type SavePlan } from './api'
import type { SessionInfo } from './gen/athena/v1/auth_pb'
import ChangePasswordDialog from './components/ChangePasswordDialog.vue'
import SignInPage from './pages/SignInPage.vue'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import MigrationDialog from './components/MigrationDialog.vue'
import { startBrowserTelemetry } from './telemetry'
import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'vue-sonner'
import type { RecordType, Page, SessionUser } from './types'
import OverviewPage from './pages/OverviewPage.vue'
import RecordTypesPage from './pages/RecordTypesPage.vue'
import PacksPage from './pages/PacksPage.vue'
import RulesPage from './pages/RulesPage.vue'
import FilesPage from './pages/FilesPage.vue'
import RecordsPage from './pages/RecordsPage.vue'

const apiState = ref<'connecting' | 'connected' | 'error'>('connecting')
const databaseState = ref('Checking')
const mode = ref<'build' | 'view'>('build')
const page = ref<Page>('overview')
const selectedRecordType = ref('')
// The open Record in View: '' for the table, 'new' for the create form, or a Record ID.
const recordRoute = ref('')
const pages: Page[] = ['overview', 'record-types', 'packs', 'rules', 'files']

// The address bar holds the current screen so a refresh or the back button
// returns to it.
function readLocation() {
  const query = new URLSearchParams(window.location.search)
  mode.value = query.get('mode') === 'view' ? 'view' : 'build'
  const requested = query.get('page')
  page.value = pages.find((item) => item === requested) ?? 'overview'
  selectedRecordType.value = query.get('record_type') ?? ''
  recordRoute.value = mode.value === 'view' ? query.get('record') ?? '' : ''
}

function writeLocation() {
  const query = new URLSearchParams()
  query.set('mode', mode.value)
  if (mode.value === 'build') query.set('page', page.value)
  if (selectedRecordType.value) query.set('record_type', selectedRecordType.value)
  if (mode.value === 'view' && recordRoute.value) query.set('record', recordRoute.value)
  const search = `?${query.toString()}`
  if (search !== window.location.search) window.history.pushState(null, '', `${window.location.pathname}${search}`)
}

readLocation()
window.addEventListener('popstate', readLocation)
const isDark = ref(false)
const workspaceName = ref('')
const recordTypes = ref<RecordType[]>([])
const emptyRecordType: RecordType = { name: '', label: '', description: '', kind: 'master', fields: [], revision: 0 }
const currentRecordType = computed(() => recordTypes.value.find((item) => item.name === selectedRecordType.value) ?? recordTypes.value[0] ?? emptyRecordType)
// View works only with saved Record Types; drafts stay in Build.
const viewRecordTypes = computed(() => recordTypes.value.filter((item) => item.revision > 0))
const viewRecordType = computed(() => viewRecordTypes.value.find((item) => item.name === selectedRecordType.value) ?? viewRecordTypes.value[0] ?? null)

// Sign-in state. The desk shows the sign-in page until the server reports a
// session; every call is checked again on the server.
const authState = ref<'checking' | 'signed-out' | 'signed-in'>('checking')
const hasUsers = ref(true)
// What the sign-in card shows when the desk cannot start normally.
const pageState = ref<Failure | null>(null)
const signInNote = ref('')
const rechecking = ref(false)
const user = ref<SessionUser | null>(null)
const changingPassword = ref(false)
// Build needs the build permission. Everyone else works in View only. Without
// a database there is no session, and the desk keeps both modes.
const canBuild = computed(() => (user.value ? user.value.systemPermissions.includes('build') : databaseState.value !== 'Connected'))
// Files has its own permissions, so the nav item follows files:read and the
// page edits only with files:write. The server checks both on every call.
const canFiles = computed(() => (user.value ? user.value.systemPermissions.includes('files:read') : false))
const canWriteFiles = computed(() => (user.value ? user.value.systemPermissions.includes('files:write') : false))
// Rules follow rules:read and the page edits only with rules:write; without a
// database there is no session and the page shows why it is unavailable.
const canRules = computed(() => (user.value ? user.value.systemPermissions.includes('rules:read') : databaseState.value !== 'Connected'))
const canWriteRules = computed(() => (user.value ? user.value.systemPermissions.includes('rules:write') : false))
// Build opens for the build permission, files:read or rules:read; a user with
// only the last two sees just those pages.
const canBuildDesk = computed(() => canBuild.value || canFiles.value || canRules.value)
const buildNav = computed<[Page, string][]>(() => {
  const items: [Page, string][] = canBuild.value ? [['overview', 'Overview'], ['record-types', 'Record Types'], ['packs', 'Packs']] : []
  if (canRules.value) items.push(['rules', 'Rules'])
  if (canFiles.value) items.push(['files', 'Files'])
  return items
})
watch(canBuildDesk, (value) => { if (!value) mode.value = 'view' })
// A user without build lands on the first Build page they have.
function landOnBuildPage() {
  const first = buildNav.value[0]?.[0]
  if (!canBuild.value && first && !buildNav.value.some(([name]) => name === page.value)) page.value = first
}
function applySession(info: SessionInfo | undefined) {
  if (info?.authenticated) {
    setCsrfToken(info.csrfToken)
    user.value = { username: info.username, displayName: info.displayName, owner: info.owner, systemPermissions: info.systemPermissions }
    authState.value = 'signed-in'
  } else {
    setCsrfToken('')
    user.value = null
    recordTypes.value = []
    authState.value = 'signed-out'
  }
}

async function loadRecordTypes() {
  try {
    recordTypes.value = await listRecordTypes()
  } catch (err) {
    toast.error(err instanceof ConnectError ? `Could not load Record Types: ${err.rawMessage}` : 'Could not load Record Types.')
  }
}

async function signedIn(info: SessionInfo) {
  signInNote.value = ''
  applySession(info)
  toast.success(`Signed in as ${info.displayName || info.username}.`)
  if (!canBuildDesk.value) mode.value = 'view'
  landOnBuildPage()
  await loadRecordTypes()
}

async function signOut() {
  try {
    await authClient.logout({})
  } catch (err) {
    toast.error(err instanceof ConnectError ? `Could not sign out: ${err.rawMessage}` : 'Could not sign out.')
    return
  }
  applySession(undefined)
  toast.success('Signed out.')
}

onSessionEnded(() => {
  if (authState.value !== 'signed-in') return
  applySession(undefined)
  signInNote.value = 'Your session ended. Sign in again.'
})

watch([mode, page, selectedRecordType, recordRoute], writeLocation)
watch(mode, (value) => { if (value === 'build') recordRoute.value = '' })

// startDesk finds out what state the server is in and opens the right page. It
// runs at start and again from "Check again" on the sign-in card.
async function startDesk() {
  pageState.value = null
  // /healthz does not need the database, so it tells a server that is down
  // from a database that is down.
  try {
    const response = await fetch('/healthz')
    const health = await response.json() as { database?: string }
    databaseState.value = health.database ?? 'Not configured'
    apiState.value = 'connected'
  } catch {
    databaseState.value = 'Unavailable'
    apiState.value = 'error'
  }
  if (apiState.value === 'error') {
    pageState.value = { kind: 'unreachable', message: 'Cannot reach the Athena server. Check that it is running, then try again.', persistent: true }
    authState.value = 'signed-out'
    return
  }
  if (databaseState.value !== 'Connected') {
    const detail = databaseState.value === 'Not configured' ? 'No database is configured. Set ATHENA_DATABASE_URL and restart Athena.' : 'The database is not connected. Start MariaDB and check ATHENA_DATABASE_URL, then try again.'
    pageState.value = { kind: 'database', message: detail, persistent: true }
    authState.value = 'signed-out'
    return
  }
  try {
    const info = await systemClient.getSystemInfo({})
    workspaceName.value = info.workspaceName
    if (info.telemetryEnabled) startBrowserTelemetry(info.version, info.workspaceSlug)
  } catch (err) {
    apiState.value = 'error'
    pageState.value = classify(err)
    authState.value = 'signed-out'
    return
  }
  try {
    const res = await authClient.getSession({})
    hasUsers.value = res.hasUsers
    applySession(res.session)
    if (!canBuildDesk.value) mode.value = 'view'
    landOnBuildPage()
    if (authState.value === 'signed-in') await loadRecordTypes()
  } catch (err) {
    const failure = classify(err)
    if (failure.persistent) pageState.value = failure
    else toast.error(describeFailure(failure))
    authState.value = 'signed-out'
  }
}
async function recheck() {
  rechecking.value = true
  try {
    await startDesk()
    if (pageState.value === null && !hasUsers.value) toast.error('Still no users. Run the command, then check again.')
  } finally {
    rechecking.value = false
  }
}
onMounted(async () => {
  const storedTheme = localStorage.getItem('athena-theme')
  isDark.value = storedTheme ? storedTheme === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.classList.toggle('dark', isDark.value)
  await startDesk()
})
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('athena-theme', isDark.value ? 'dark' : 'light')
}

function addRecordType() {
  const name = `record_type_${recordTypes.value.length + 1}`
  recordTypes.value.push({ name, label: 'Untitled Record Type', description: '', kind: 'master', fields: [], revision: 0 })
  selectedRecordType.value = name
  page.value = 'record-types'
}
function renameRecordType(from: string, to: string) {
  const recordType = recordTypes.value.find((item) => item.name === from)
  if (!recordType || recordType.revision > 0) return
  recordType.name = to
  if (selectedRecordType.value === from) selectedRecordType.value = to
}
async function removeRecordType(recordType: RecordType) {
  if (recordType.revision > 0) {
    try {
      await deleteRecordType(recordType)
    } catch (err) {
      toast.error(err instanceof ConnectError ? `Could not delete: ${err.rawMessage}` : 'Could not delete this Record Type.')
      return
    }
  }
  recordTypes.value = recordTypes.value.filter((item) => item !== recordType)
  selectedRecordType.value = recordTypes.value[0]?.name ?? ''
  toast.success(recordType.revision > 0 ? `${recordType.label} deleted.` : `${recordType.label} discarded.`)
}
function addField() {
  currentRecordType.value.fields.push({ name: `field_${currentRecordType.value.fields.length + 1}`, label: 'New field', type: 'Text', required: false })
}
// A save that would delete values or needs fill-in values waits here for the
// person to review its plan in MigrationDialog.
const pendingSave = ref<{ recordType: RecordType; plan: SavePlan; complete?: (saved: boolean) => void } | null>(null)

function saveError(err: unknown) {
  toast.error(err instanceof ConnectError ? `Could not save: ${err.rawMessage}` : 'Could not save this Record Type.')
}

async function saveCurrentRecordType(complete?: (saved: boolean) => void) {
  if (databaseState.value !== 'Connected') {
    toast.error('The database is not connected. This change is only in this browser session.')
    complete?.(false)
    return
  }
  const recordType = currentRecordType.value
  if (recordType.revision > 0) {
    try {
      const plan = await planRecordTypeSave(recordType)
      if (plan.requiresConfirmation) {
        pendingSave.value = { recordType, plan, complete }
        return
      }
    } catch (err) {
      saveError(err)
      complete?.(false)
      return
    }
  }
  await commitSave(recordType, undefined, complete)
}

async function commitSave(recordType: RecordType, answers: SaveAnswers | undefined, complete?: (saved: boolean) => void) {
  let saved = false
  try {
    // A saved parent whose formulas read this child's fields is saved in the
    // same request, so its rollup needs no second save.
    const parent = recordType.kind === 'child' ? recordTypes.value.find((item) => item.name === recordType.parent_record_type && item.revision > 0 && item.fields.some((field) => field.formula?.includes(`doc.${recordType.name}.`))) : undefined
    const result = await saveRecordType(recordType, answers, parent)
    recordType.revision = result.saved.revision
    if (parent && result.parent) parent.revision = result.parent.revision
    toast.success(parent ? `${recordType.label} and ${parent.label} saved.` : `${recordType.label} saved.`)
    saved = true
  } catch (err) {
    saveError(err)
  }
  complete?.(saved)
}

function confirmPendingSave(answers: SaveAnswers) {
  const pending = pendingSave.value
  pendingSave.value = null
  if (pending) void commitSave(pending.recordType, answers, pending.complete)
}

function cancelPendingSave() {
  pendingSave.value?.complete?.(false)
  pendingSave.value = null
}
</script>

<template>
  <SignInPage v-if="authState === 'signed-out'" :has-users="hasUsers" :workspace-name="workspaceName" :page-state="pageState" :note="signInNote" :checking="rechecking" @signed-in="signedIn" @recheck="recheck" />
  <div v-else-if="authState === 'signed-in'" class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="#overview" @click.prevent="page = 'overview'; mode = 'build'">Athena<small v-if="workspaceName" class="brand-project">{{ workspaceName }}</small></a>
      <nav v-if="mode === 'build'" class="primary-nav" aria-label="Build">
        <button v-for="item in buildNav" :key="item[0]" class="nav-item" :class="{ active: page === item[0] }" @click="page = item[0]">{{ item[1] }}<span v-if="item[0] === 'record-types'" class="nav-count">{{ recordTypes.length }}</span></button>
      </nav>
      <nav v-else class="primary-nav" aria-label="Record Types">
        <span class="nav-heading">Records</span>
        <button v-for="recordType in viewRecordTypes" :key="recordType.name" class="nav-item" :class="{ active: viewRecordType?.name === recordType.name }" @click="selectedRecordType = recordType.name; recordRoute = ''">{{ recordType.label }}</button>
        <p v-if="viewRecordTypes.length === 0" class="nav-empty">No saved Record Types.</p>
      </nav>
    </aside>
    <main class="main-content">
      <header class="topbar"><div class="topbar-actions"><div v-if="canBuildDesk" class="mode-switch" aria-label="Desk mode"><button :class="{ selected: mode === 'build' }" @click="mode = 'build'">Build</button><button :class="{ selected: mode === 'view' }" @click="mode = 'view'">View</button></div><Button variant="ghost" size="icon-sm" class="theme-toggle" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'" :title="isDark ? 'Switch to light theme' : 'Switch to dark theme'" @click="toggleTheme"><span aria-hidden="true">{{ isDark ? '☼' : '◐' }}</span></Button><DropdownMenu v-if="user"><DropdownMenuTrigger as-child><Button variant="ghost" size="sm" class="user-menu-trigger">{{ user.displayName || user.username }}</Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuLabel class="user-menu-name">{{ user.displayName || user.username }}<small>{{ user.owner ? 'Instance owner' : user.username }}</small></DropdownMenuLabel><DropdownMenuSeparator /><DropdownMenuItem @select="changingPassword = true">Change password</DropdownMenuItem><DropdownMenuItem @select="signOut">Sign out</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div></header>
      <OverviewPage v-if="mode === 'build' && page === 'overview'" :record-types="recordTypes" :database-state="databaseState" :api-state="apiState" @navigate="page = $event" @create-record-type="page = 'record-types'; addRecordType()" />
      <RecordTypesPage v-else-if="mode === 'build' && page === 'record-types'" :record-types="recordTypes" :current-record-type="currentRecordType" :database-state="databaseState" @select="selectedRecordType = $event" @add-record-type="addRecordType" @add-field="addField" @rename="renameRecordType" @delete="removeRecordType" @save="saveCurrentRecordType" @preview="mode = 'view'" />
      <PacksPage v-else-if="mode === 'build' && page === 'packs'" @open-record-types="page = 'record-types'" />
      <RulesPage v-else-if="mode === 'build' && page === 'rules'" :can-write="canWriteRules" :record-types="recordTypes" />
      <FilesPage v-else-if="mode === 'build' && page === 'files'" :can-write="canWriteFiles" />
      <RecordsPage v-else v-model:record-id="recordRoute" :record-type="viewRecordType" :record-types="viewRecordTypes" @back="mode = 'build'" />
    </main>
    <MigrationDialog v-if="pendingSave" :record-type="pendingSave.recordType" :plan="pendingSave.plan" @confirm="confirmPendingSave" @cancel="cancelPendingSave" />
    <ChangePasswordDialog v-model:open="changingPassword" />
  </div>
  <Toaster position="bottom-right" :theme="isDark ? 'dark' : 'light'" />
</template>
