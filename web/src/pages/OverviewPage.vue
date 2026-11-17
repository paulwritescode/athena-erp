<script setup lang="ts">
import type { FormDefinition, Page } from '../types'
defineProps<{ forms: FormDefinition[]; databaseState: string; apiState: string; productName: string; version: string }>()
const emit = defineEmits<{ navigate: [page: Page]; createForm: [] }>()
const modules = [{ label: 'Core', forms: 1 }, { label: 'Invoicing', forms: 1 }, { label: 'Support', forms: 1 }]
</script>

<template>
  <section class="page-content">
    <div class="welcome-row"><div><div class="eyebrow">CONTROL DESK</div><h1>Build your workspace</h1><p class="lead">Define the records, capabilities, and rules your business needs.</p></div><button class="button primary" @click="emit('createForm')">＋ New Form</button></div>
    <div class="status-banner"><div class="status-symbol">⌘</div><div class="status-copy"><strong>Build the system your team will use</strong><p>Use Build to configure Forms, group them into installable Modules, and define business rules. Switch to View to see the working experience.</p></div><span class="api-pill" :class="`is-${apiState}`"><i></i>{{ apiState === 'connected' ? 'API connected' : 'API offline' }}</span></div>
    <div class="section-heading"><div><h2>Workspace building blocks</h2><p>Start with a record, then compose a business capability</p></div></div>
    <div class="foundation-grid">
      <button class="foundation-card" @click="emit('navigate', 'forms')"><div class="card-top"><span class="card-icon">▤</span><span class="card-tag">RECORD DEFINITIONS</span></div><h3>Forms</h3><p>Define the fields and behavior of records such as Customers, Invoices, and Tickets.</p><div class="card-footer"><span class="muted-label">{{ forms.length }} sample definitions</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'modules')"><div class="card-top"><span class="card-icon">▦</span><span class="card-tag">INSTALLABLE FEATURES</span></div><h3>Modules</h3><p>Group related Forms, rules, reports, and workflows into reusable business features.</p><div class="card-footer"><span class="muted-label">{{ modules.length }} starter modules</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'rules')"><div class="card-top"><span class="card-icon">⌘</span><span class="card-tag">CONFIGURABLE LOGIC</span></div><h3>Rules</h3><p>Use Go for trusted business logic; Starlark is planned for configurable conditions and actions.</p><div class="card-footer"><span class="muted-label">Go core · Starlark planned</span><span class="arrow">→</span></div></button>
    </div>
    <div class="lower-grid"><section class="panel"><div class="panel-heading"><div><h2>Form definitions</h2><p>Open a definition in the form builder</p></div><button class="text-button" @click="emit('navigate', 'forms')">View all →</button></div><button v-for="form in forms" :key="form.name" class="list-row" @click="emit('navigate', 'forms')"><span class="list-icon">▤</span><span class="row-main"><strong>{{ form.label }}</strong><small>{{ form.module }} · {{ form.fields.length }} fields</small></span><span class="row-meta">Definition</span><span class="arrow">→</span></button></section>
      <section class="panel"><div class="panel-heading"><div><h2>Instance status</h2><p>Connection to this Athena server</p></div><span class="live-indicator"><i></i>{{ apiState === 'connected' ? 'LIVE' : 'OFFLINE' }}</span></div><div class="server-details"><div><span>Frontend</span><strong>Vue 3 · TypeScript · Vite</strong></div><div><span>API</span><strong>Go · ConnectRPC</strong></div><div><span>Server</span><strong>Same origin · port 3000</strong></div><div><span>MariaDB</span><strong>{{ databaseState }}</strong></div></div><p class="database-hint">Set <code>ATHENA_DATABASE_URL</code> to connect MariaDB.</p></section></div>
  </section>
</template>
