<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { SystemService } from './gen/athena/v1/system_pb'
import { Button } from '@/components/ui/button'
import type { RecordDefinition, Page } from './types'
import OverviewPage from './pages/OverviewPage.vue'
import RecordsPage from './pages/RecordsPage.vue'
import ModulesPage from './pages/ModulesPage.vue'
import RulesPage from './pages/RulesPage.vue'
import RecordViewPage from './pages/RecordViewPage.vue'

const systemClient = createClient(SystemService, createConnectTransport({ baseUrl: window.location.origin }))
const apiState = ref<'connecting' | 'connected' | 'error'>('connecting')
const databaseState = ref('Checking')
const productName = ref('Athena ERP')
const version = ref('')
const mode = ref<'build' | 'view'>('build')
const page = ref<Page>('overview')
const selectedRecord = ref('')
const savedNotice = ref('')
const isDark = ref(false)
const records = ref<RecordDefinition[]>([])
const emptyRecord: RecordDefinition = { name: '', label: '', description: '', fields: [] }
const currentRecord = computed(() => records.value.find((item) => item.name === selectedRecord.value) ?? records.value[0] ?? emptyRecord)
const heading = computed(() => mode.value === 'view' ? currentRecord.value.label : ({ overview: 'Overview', records: 'Records', modules: 'Modules', rules: 'Rules' })[page.value])

onMounted(async () => {
  const storedTheme = localStorage.getItem('athena-theme')
  isDark.value = storedTheme ? storedTheme === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.classList.toggle('dark', isDark.value)
  try {
    const info = await systemClient.getSystemInfo({})
    productName.value = info.productName
    version.value = info.version
    apiState.value = 'connected'
  } catch { apiState.value = 'error' }
  try {
    const response = await fetch('/healthz')
    const health = await response.json() as { database?: string }
    databaseState.value = health.database ?? 'Not configured'
    if (databaseState.value === 'Connected') {
      const stored = await fetch('/api/records')
      if (stored.ok) {
        const definitions = await stored.json() as RecordDefinition[]
        if (definitions.length > 0) records.value = definitions
      }
    }
  } catch { databaseState.value = 'Unavailable' }
})

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('athena-theme', isDark.value ? 'dark' : 'light')
}

function addRecord() {
  const name = `record_${records.value.length + 1}`
  records.value.push({ name, label: 'Untitled Record', description: '', fields: [] })
  selectedRecord.value = name
  page.value = 'records'
}
function addField() {
  currentRecord.value.fields.push({ name: `field_${currentRecord.value.fields.length + 1}`, label: 'New field', type: 'Text', required: false })
}
async function saveCurrentRecord() {
  if (databaseState.value !== 'Connected') savedNotice.value = 'MariaDB is not connected. This change is only in this browser session.'
  else {
    try {
      const response = await fetch('/api/records', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(currentRecord.value) })
      if (!response.ok) throw new Error(await response.text())
      savedNotice.value = 'Record saved to MariaDB.'
    } catch { savedNotice.value = 'Could not save this Record to MariaDB.' }
  }
  window.setTimeout(() => { savedNotice.value = '' }, 3000)
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="#overview" @click.prevent="page = 'overview'; mode = 'build'"><span class="brand-mark">A</span><span class="brand-copy"><strong>Athena</strong><small>ERP workspace</small></span></a>
      <div class="workspace-label">WORKSPACE</div>
      <nav class="primary-nav" aria-label="Main navigation">
        <button v-for="item in ([['overview', 'Overview', '⌂'], ['records', 'Records', '▤'], ['modules', 'Modules', '▦'], ['rules', 'Rules', '⌘']] as [Page, string, string][])" :key="item[0]" class="nav-item" :class="{ active: mode === 'build' && page === item[0] }" @click="mode = 'build'; page = item[0]"><span class="nav-icon">{{ item[2] }}</span>{{ item[1] }}<span v-if="item[0] === 'records'" class="nav-count">{{ records.length }}</span></button>
      </nav>
      <div class="sidebar-bottom"><div class="connection-card"><span class="connection-dot" :class="`is-${apiState}`"></span><div><strong>{{ apiState === 'connected' ? 'Server connected' : apiState === 'error' ? 'Server unavailable' : 'Connecting' }}</strong><small>Go · ConnectRPC</small></div></div><div class="user-row"><span class="avatar">A</span><span><strong>Administrator</strong><small>Local workspace</small></span><span class="more">•••</span></div></div>
    </aside>
    <main class="main-content">
      <header class="topbar"><div class="breadcrumb"><span>Workspace</span><span class="crumb-slash">/</span><strong>{{ heading }}</strong></div><div class="topbar-actions"><div class="mode-switch" aria-label="Workspace mode"><button :class="{ selected: mode === 'build' }" @click="mode = 'build'">Build</button><button :class="{ selected: mode === 'view' }" @click="mode = 'view'">View</button></div><span class="environment-badge"><i></i>Local</span><Button variant="ghost" size="icon-sm" class="theme-toggle" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'" :title="isDark ? 'Switch to light theme' : 'Switch to dark theme'" @click="toggleTheme"><span aria-hidden="true">{{ isDark ? '☼' : '◐' }}</span></Button></div></header>
      <OverviewPage v-if="mode === 'build' && page === 'overview'" :records="records" :database-state="databaseState" :api-state="apiState" @navigate="page = $event" @create-record="page = 'records'; addRecord()" />
      <RecordsPage v-else-if="mode === 'build' && page === 'records'" :records="records" :current-record="currentRecord" :database-state="databaseState" @select="selectedRecord = $event" @add-record="addRecord" @add-field="addField" @save="saveCurrentRecord" @preview="mode = 'view'" />
      <ModulesPage v-else-if="mode === 'build' && page === 'modules'" @open-records="page = 'records'" />
      <RulesPage v-else-if="mode === 'build' && page === 'rules'" />
      <RecordViewPage v-else :records="records" :current-record="currentRecord" :notice="savedNotice" @select="selectedRecord = $event" @back="mode = 'build'" @save="saveCurrentRecord" />
      <footer class="page-footer"><span>{{ productName }} <span v-if="version">· {{ version }}</span></span><span>Build the definition. View the experience.</span></footer>
    </main>
  </div>
</template>
