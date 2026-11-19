<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { deleteRecord, listRecords, planRecordSave, saveRecord, systemClient, type SaveAnswers, type SavePlan } from './api'
import MigrationDialog from './components/MigrationDialog.vue'
import { startBrowserTelemetry } from './telemetry'
import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'vue-sonner'
import type { RecordDefinition, Page } from './types'
import OverviewPage from './pages/OverviewPage.vue'
import RecordsPage from './pages/RecordsPage.vue'
import ModulesPage from './pages/ModulesPage.vue'
import RulesPage from './pages/RulesPage.vue'
import EntriesPage from './pages/EntriesPage.vue'

const apiState = ref<'connecting' | 'connected' | 'error'>('connecting')
const databaseState = ref('Checking')
const mode = ref<'build' | 'view'>('build')
const page = ref<Page>('overview')
const selectedRecord = ref('')
// The open entry in View: '' for the table, 'new' for the create form, or an entry ID.
const entryRoute = ref('')
const pages: Page[] = ['overview', 'records', 'modules', 'rules']

// The address bar holds the current screen so a refresh or the back button
// returns to it.
function readLocation() {
  const query = new URLSearchParams(window.location.search)
  mode.value = query.get('mode') === 'view' ? 'view' : 'build'
  const requested = query.get('page')
  page.value = pages.find((item) => item === requested) ?? 'overview'
  selectedRecord.value = query.get('record') ?? ''
  entryRoute.value = mode.value === 'view' ? query.get('entry') ?? '' : ''
}

function writeLocation() {
  const query = new URLSearchParams()
  query.set('mode', mode.value)
  if (mode.value === 'build') query.set('page', page.value)
  if (selectedRecord.value) query.set('record', selectedRecord.value)
  if (mode.value === 'view' && entryRoute.value) query.set('entry', entryRoute.value)
  const search = `?${query.toString()}`
  if (search !== window.location.search) window.history.pushState(null, '', `${window.location.pathname}${search}`)
}

readLocation()
window.addEventListener('popstate', readLocation)
const isDark = ref(false)
const projectName = ref('')
const records = ref<RecordDefinition[]>([])
const emptyRecord: RecordDefinition = { name: '', label: '', description: '', fields: [], revision: 0 }
const currentRecord = computed(() => records.value.find((item) => item.name === selectedRecord.value) ?? records.value[0] ?? emptyRecord)
// View works only with saved Records; drafts stay in Build.
const viewRecords = computed(() => records.value.filter((item) => item.revision > 0))
const viewRecord = computed(() => viewRecords.value.find((item) => item.name === selectedRecord.value) ?? viewRecords.value[0] ?? null)

watch([mode, page, selectedRecord, entryRoute], writeLocation)
watch(mode, (value) => { if (value === 'build') entryRoute.value = '' })

onMounted(async () => {
  const storedTheme = localStorage.getItem('athena-theme')
  isDark.value = storedTheme ? storedTheme === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.classList.toggle('dark', isDark.value)
  try {
    const info = await systemClient.getSystemInfo({})
    projectName.value = info.projectName
    if (info.telemetryEnabled) startBrowserTelemetry(info.version, info.projectSlug)
    apiState.value = 'connected'
  } catch { apiState.value = 'error' }
  try {
    const response = await fetch('/healthz')
    const health = await response.json() as { database?: string }
    databaseState.value = health.database ?? 'Not configured'
    if (databaseState.value === 'Connected') {
      records.value = await listRecords()
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

function addRecord() {
  const name = `record_${records.value.length + 1}`
  records.value.push({ name, label: 'Untitled Record', description: '', fields: [], revision: 0 })
  selectedRecord.value = name
  page.value = 'records'
}
function renameRecord(from: string, to: string) {
  const record = records.value.find((item) => item.name === from)
  if (!record || record.revision > 0) return
  record.name = to
  if (selectedRecord.value === from) selectedRecord.value = to
}
async function removeRecord(record: RecordDefinition) {
  if (record.revision > 0) {
    try {
      await deleteRecord(record)
    } catch (err) {
      toast.error(err instanceof ConnectError ? `Could not delete: ${err.rawMessage}` : 'Could not delete this Record.')
      return
    }
  }
  records.value = records.value.filter((item) => item !== record)
  selectedRecord.value = records.value[0]?.name ?? ''
  toast.success(record.revision > 0 ? `${record.label} deleted.` : `${record.label} discarded.`)
}
function addField() {
  currentRecord.value.fields.push({ name: `field_${currentRecord.value.fields.length + 1}`, label: 'New field', type: 'Text', required: false })
}
// A save that would delete values or needs fill-in values waits here for the
// person to review its plan in MigrationDialog.
const pendingSave = ref<{ record: RecordDefinition; plan: SavePlan; complete?: (saved: boolean) => void } | null>(null)

function saveError(err: unknown) {
  toast.error(err instanceof ConnectError ? `Could not save: ${err.rawMessage}` : 'Could not save this Record.')
}

async function saveCurrentRecord(complete?: (saved: boolean) => void) {
  if (databaseState.value !== 'Connected') {
    toast.error('The database is not connected. This change is only in this browser session.')
    complete?.(false)
    return
  }
  const record = currentRecord.value
  if (record.revision > 0) {
    try {
      const plan = await planRecordSave(record)
      if (plan.requiresConfirmation) {
        pendingSave.value = { record, plan, complete }
        return
      }
    } catch (err) {
      saveError(err)
      complete?.(false)
      return
    }
  }
  await commitSave(record, undefined, complete)
}

async function commitSave(record: RecordDefinition, answers: SaveAnswers | undefined, complete?: (saved: boolean) => void) {
  let saved = false
  try {
    const result = await saveRecord(record, answers)
    record.revision = result.revision
    toast.success(`${record.label} saved.`)
    saved = true
  } catch (err) {
    saveError(err)
  }
  complete?.(saved)
}

function confirmPendingSave(answers: SaveAnswers) {
  const pending = pendingSave.value
  pendingSave.value = null
  if (pending) void commitSave(pending.record, answers, pending.complete)
}

function cancelPendingSave() {
  pendingSave.value?.complete?.(false)
  pendingSave.value = null
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="#overview" @click.prevent="page = 'overview'; mode = 'build'">Athena<small v-if="projectName" class="brand-project">{{ projectName }}</small></a>
      <nav v-if="mode === 'build'" class="primary-nav" aria-label="Build">
        <button v-for="item in ([['overview', 'Overview'], ['records', 'Records'], ['modules', 'Modules'], ['rules', 'Rules']] as [Page, string][])" :key="item[0]" class="nav-item" :class="{ active: page === item[0] }" @click="page = item[0]">{{ item[1] }}<span v-if="item[0] === 'records'" class="nav-count">{{ records.length }}</span></button>
      </nav>
      <nav v-else class="primary-nav" aria-label="Records">
        <span class="nav-heading">Records</span>
        <button v-for="record in viewRecords" :key="record.name" class="nav-item" :class="{ active: viewRecord?.name === record.name }" @click="selectedRecord = record.name; entryRoute = ''">{{ record.label }}</button>
        <p v-if="viewRecords.length === 0" class="nav-empty">No saved Records.</p>
      </nav>
    </aside>
    <main class="main-content">
      <header class="topbar"><div class="topbar-actions"><div class="mode-switch" aria-label="Workspace mode"><button :class="{ selected: mode === 'build' }" @click="mode = 'build'">Build</button><button :class="{ selected: mode === 'view' }" @click="mode = 'view'">View</button></div><Button variant="ghost" size="icon-sm" class="theme-toggle" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'" :title="isDark ? 'Switch to light theme' : 'Switch to dark theme'" @click="toggleTheme"><span aria-hidden="true">{{ isDark ? '☼' : '◐' }}</span></Button></div></header>
      <OverviewPage v-if="mode === 'build' && page === 'overview'" :records="records" :database-state="databaseState" :api-state="apiState" @navigate="page = $event" @create-record="page = 'records'; addRecord()" />
      <RecordsPage v-else-if="mode === 'build' && page === 'records'" :records="records" :current-record="currentRecord" :database-state="databaseState" @select="selectedRecord = $event" @add-record="addRecord" @add-field="addField" @rename="renameRecord" @delete="removeRecord" @save="saveCurrentRecord" @preview="mode = 'view'" />
      <ModulesPage v-else-if="mode === 'build' && page === 'modules'" @open-records="page = 'records'" />
      <RulesPage v-else-if="mode === 'build' && page === 'rules'" />
      <EntriesPage v-else v-model:entry-id="entryRoute" :record="viewRecord" @back="mode = 'build'" />
    </main>
    <MigrationDialog v-if="pendingSave" :record="pendingSave.record" :plan="pendingSave.plan" @confirm="confirmPendingSave" @cancel="cancelPendingSave" />
    <Toaster position="bottom-right" :theme="isDark ? 'dark' : 'light'" />
  </div>
</template>
