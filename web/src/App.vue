<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { SystemService } from './gen/athena/v1/system_pb'
import type { FormDefinition, Page } from './types'
import OverviewPage from './pages/OverviewPage.vue'
import FormsPage from './pages/FormsPage.vue'
import ModulesPage from './pages/ModulesPage.vue'
import RulesPage from './pages/RulesPage.vue'
import ViewPage from './pages/ViewPage.vue'

const systemClient = createClient(SystemService, createConnectTransport({ baseUrl: window.location.origin }))
const apiState = ref<'connecting' | 'connected' | 'error'>('connecting')
const databaseState = ref('Checking')
const productName = ref('Athena ERP')
const version = ref('')
const mode = ref<'build' | 'view'>('build')
const page = ref<Page>('overview')
const selectedForm = ref('Customer')
const savedNotice = ref('')
const forms = ref<FormDefinition[]>([
  { name: 'Customer', label: 'Customer', description: 'A person or organization your business serves.', module: 'Core', fields: [
    { name: 'customer_name', label: 'Customer name', type: 'Text', required: true }, { name: 'customer_type', label: 'Customer type', type: 'Select', required: true }, { name: 'email', label: 'Email address', type: 'Email', required: false }, { name: 'phone', label: 'Phone number', type: 'Phone', required: false }, { name: 'tax_number', label: 'Tax number', type: 'Text', required: false },
  ] },
  { name: 'SalesInvoice', label: 'Sales Invoice', description: 'A request for payment issued to a customer.', module: 'Invoicing', fields: [
    { name: 'customer', label: 'Customer', type: 'Link', required: true }, { name: 'posting_date', label: 'Posting date', type: 'Date', required: true }, { name: 'due_date', label: 'Due date', type: 'Date', required: true }, { name: 'currency', label: 'Currency', type: 'Select', required: true }, { name: 'total', label: 'Total', type: 'Currency', required: true },
  ] },
  { name: 'Ticket', label: 'Support Ticket', description: 'A customer request tracked through resolution.', module: 'Support', fields: [
    { name: 'subject', label: 'Subject', type: 'Text', required: true }, { name: 'requester', label: 'Requester', type: 'Link', required: true }, { name: 'priority', label: 'Priority', type: 'Select', required: true }, { name: 'status', label: 'Status', type: 'Select', required: true }, { name: 'description', label: 'Description', type: 'Long text', required: false },
  ] },
])
const currentForm = computed(() => forms.value.find((item) => item.name === selectedForm.value) ?? forms.value[0]!)
const heading = computed(() => mode.value === 'view' ? currentForm.value.label : ({ overview: 'Overview', forms: 'Forms', modules: 'Modules', rules: 'Rules' })[page.value])

onMounted(async () => {
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
      const stored = await fetch('/api/forms')
      if (stored.ok) {
        const definitions = await stored.json() as FormDefinition[]
        if (definitions.length > 0) forms.value = definitions
      }
    }
  } catch { databaseState.value = 'Unavailable' }
})

function addForm() {
  const name = `NewForm${forms.value.length}`
  forms.value.push({ name, label: 'Untitled Form', description: 'Describe what this record is for.', module: 'Core', fields: [{ name: 'title', label: 'Title', type: 'Text', required: true }] })
  selectedForm.value = name
  page.value = 'forms'
}
function addField() {
  currentForm.value.fields.push({ name: `field_${currentForm.value.fields.length + 1}`, label: 'New field', type: 'Text', required: false })
}
async function saveCurrentForm() {
  if (databaseState.value !== 'Connected') savedNotice.value = 'MariaDB is not connected. This change is only in this browser session.'
  else {
    try {
      const response = await fetch('/api/forms', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(currentForm.value) })
      if (!response.ok) throw new Error(await response.text())
      savedNotice.value = 'Form saved to MariaDB.'
    } catch { savedNotice.value = 'Could not save this Form to MariaDB.' }
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
        <button v-for="item in ([['overview', 'Overview', '⌂'], ['forms', 'Forms', '▤'], ['modules', 'Modules', '▦'], ['rules', 'Rules', '⌘']] as [Page, string, string][])" :key="item[0]" class="nav-item" :class="{ active: mode === 'build' && page === item[0] }" @click="mode = 'build'; page = item[0]"><span class="nav-icon">{{ item[2] }}</span>{{ item[1] }}<span v-if="item[0] === 'forms'" class="nav-count">{{ forms.length }}</span></button>
      </nav>
      <div class="sidebar-bottom"><div class="connection-card"><span class="connection-dot" :class="`is-${apiState}`"></span><div><strong>{{ apiState === 'connected' ? 'Server connected' : apiState === 'error' ? 'Server unavailable' : 'Connecting' }}</strong><small>Go · ConnectRPC</small></div></div><div class="user-row"><span class="avatar">A</span><span><strong>Administrator</strong><small>Local workspace</small></span><span class="more">•••</span></div></div>
    </aside>
    <main class="main-content">
      <header class="topbar"><div class="breadcrumb"><span>Workspace</span><span class="crumb-slash">/</span><strong>{{ heading }}</strong></div><div class="topbar-actions"><div class="mode-switch" aria-label="Workspace mode"><button :class="{ selected: mode === 'build' }" @click="mode = 'build'">Build</button><button :class="{ selected: mode === 'view' }" @click="mode = 'view'">View</button></div><span class="environment-badge"><i></i>Local</span></div></header>
      <OverviewPage v-if="mode === 'build' && page === 'overview'" :forms="forms" :database-state="databaseState" :api-state="apiState" :product-name="productName" :version="version" @navigate="page = $event" @create-form="page = 'forms'; addForm()" />
      <FormsPage v-else-if="mode === 'build' && page === 'forms'" :forms="forms" :current-form="currentForm" :database-state="databaseState" @select="selectedForm = $event" @add-form="addForm" @add-field="addField" @save="saveCurrentForm" @preview="mode = 'view'" />
      <ModulesPage v-else-if="mode === 'build' && page === 'modules'" @open-forms="page = 'forms'" />
      <RulesPage v-else-if="mode === 'build' && page === 'rules'" />
      <ViewPage v-else :forms="forms" :current-form="currentForm" :notice="savedNotice" @select="selectedForm = $event" @back="mode = 'build'" @save="saveCurrentForm" />
      <footer class="page-footer"><span>{{ productName }} <span v-if="version">· {{ version }}</span></span><span>Build the definition. View the experience.</span></footer>
    </main>
  </div>
</template>
