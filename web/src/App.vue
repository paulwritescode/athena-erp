<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { deleteRecordType, listRecordTypes, planRecordTypeSave, saveRecordType, systemClient, type SaveAnswers, type SavePlan } from './api'
import MigrationDialog from './components/MigrationDialog.vue'
import { startBrowserTelemetry } from './telemetry'
import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'vue-sonner'
import type { RecordType, Page } from './types'
import OverviewPage from './pages/OverviewPage.vue'
import RecordTypesPage from './pages/RecordTypesPage.vue'
import PacksPage from './pages/PacksPage.vue'
import RulesPage from './pages/RulesPage.vue'
import RecordsPage from './pages/RecordsPage.vue'

const apiState = ref<'connecting' | 'connected' | 'error'>('connecting')
const databaseState = ref('Checking')
const mode = ref<'build' | 'view'>('build')
const page = ref<Page>('overview')
const selectedRecordType = ref('')
// The open Record in View: '' for the table, 'new' for the create form, or a Record ID.
const recordRoute = ref('')
const pages: Page[] = ['overview', 'record-types', 'packs', 'rules']

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
const emptyRecordType: RecordType = { name: '', label: '', description: '', fields: [], revision: 0 }
const currentRecordType = computed(() => recordTypes.value.find((item) => item.name === selectedRecordType.value) ?? recordTypes.value[0] ?? emptyRecordType)
// View works only with saved Record Types; drafts stay in Build.
const viewRecordTypes = computed(() => recordTypes.value.filter((item) => item.revision > 0))
const viewRecordType = computed(() => viewRecordTypes.value.find((item) => item.name === selectedRecordType.value) ?? viewRecordTypes.value[0] ?? null)

watch([mode, page, selectedRecordType, recordRoute], writeLocation)
watch(mode, (value) => { if (value === 'build') recordRoute.value = '' })

onMounted(async () => {
  const storedTheme = localStorage.getItem('athena-theme')
  isDark.value = storedTheme ? storedTheme === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.classList.toggle('dark', isDark.value)
  try {
    const info = await systemClient.getSystemInfo({})
    workspaceName.value = info.workspaceName
    if (info.telemetryEnabled) startBrowserTelemetry(info.version, info.workspaceSlug)
    apiState.value = 'connected'
  } catch { apiState.value = 'error' }
  try {
    const response = await fetch('/healthz')
    const health = await response.json() as { database?: string }
    databaseState.value = health.database ?? 'Not configured'
    if (databaseState.value === 'Connected') {
      recordTypes.value = await listRecordTypes()
    }
  } catch { databaseState.value = 'Unavailable' }
  // The sidebar has no status line, so report a missing connection once here.
  if (apiState.value === 'error') toast.error('The Athena server is unavailable.')
  else if (databaseState.value !== 'Connected') toast.error(`Database: ${databaseState.value}. Changes stay in this browser session.`)
})

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('athena-theme', isDark.value ? 'dark' : 'light')
}

function addRecordType() {
  const name = `record_type_${recordTypes.value.length + 1}`
  recordTypes.value.push({ name, label: 'Untitled Record Type', description: '', fields: [], revision: 0 })
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
    const result = await saveRecordType(recordType, answers)
    recordType.revision = result.revision
    toast.success(`${recordType.label} saved.`)
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
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="#overview" @click.prevent="page = 'overview'; mode = 'build'">Athena<small v-if="workspaceName" class="brand-project">{{ workspaceName }}</small></a>
      <nav v-if="mode === 'build'" class="primary-nav" aria-label="Build">
        <button v-for="item in ([['overview', 'Overview'], ['record-types', 'Record Types'], ['packs', 'Packs'], ['rules', 'Rules']] as [Page, string][])" :key="item[0]" class="nav-item" :class="{ active: page === item[0] }" @click="page = item[0]">{{ item[1] }}<span v-if="item[0] === 'record-types'" class="nav-count">{{ recordTypes.length }}</span></button>
      </nav>
      <nav v-else class="primary-nav" aria-label="Record Types">
        <span class="nav-heading">Records</span>
        <button v-for="recordType in viewRecordTypes" :key="recordType.name" class="nav-item" :class="{ active: viewRecordType?.name === recordType.name }" @click="selectedRecordType = recordType.name; recordRoute = ''">{{ recordType.label }}</button>
        <p v-if="viewRecordTypes.length === 0" class="nav-empty">No saved Record Types.</p>
      </nav>
    </aside>
    <main class="main-content">
      <header class="topbar"><div class="topbar-actions"><div class="mode-switch" aria-label="Desk mode"><button :class="{ selected: mode === 'build' }" @click="mode = 'build'">Build</button><button :class="{ selected: mode === 'view' }" @click="mode = 'view'">View</button></div><Button variant="ghost" size="icon-sm" class="theme-toggle" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'" :title="isDark ? 'Switch to light theme' : 'Switch to dark theme'" @click="toggleTheme"><span aria-hidden="true">{{ isDark ? '☼' : '◐' }}</span></Button></div></header>
      <OverviewPage v-if="mode === 'build' && page === 'overview'" :record-types="recordTypes" :database-state="databaseState" :api-state="apiState" @navigate="page = $event" @create-record-type="page = 'record-types'; addRecordType()" />
      <RecordTypesPage v-else-if="mode === 'build' && page === 'record-types'" :record-types="recordTypes" :current-record-type="currentRecordType" :database-state="databaseState" @select="selectedRecordType = $event" @add-record-type="addRecordType" @add-field="addField" @rename="renameRecordType" @delete="removeRecordType" @save="saveCurrentRecordType" @preview="mode = 'view'" />
      <PacksPage v-else-if="mode === 'build' && page === 'packs'" @open-record-types="page = 'record-types'" />
      <RulesPage v-else-if="mode === 'build' && page === 'rules'" />
      <RecordsPage v-else v-model:record-id="recordRoute" :record-type="viewRecordType" @back="mode = 'build'" />
    </main>
    <MigrationDialog v-if="pendingSave" :record-type="pendingSave.recordType" :plan="pendingSave.plan" @confirm="confirmPendingSave" @cancel="cancelPendingSave" />
    <Toaster position="bottom-right" :theme="isDark ? 'dark' : 'light'" />
  </div>
</template>
