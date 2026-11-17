<script setup lang="ts">
import type { RecordDefinition, Page } from '../types'
import { Button } from '@/components/ui/button'
defineProps<{ records: RecordDefinition[]; databaseState: string; apiState: string }>()
const emit = defineEmits<{ navigate: [page: Page]; createRecord: [] }>()
</script>

<template>
  <section class="page-content">
    <div class="welcome-row"><div><div class="eyebrow">PLATFORM CONTROL DESK</div><h1>Build your workspace</h1><p class="lead">Define the Records and reusable capabilities your ERP needs.</p></div><Button @click="emit('createRecord')">＋ New Record</Button></div>
    <div class="status-banner"><div class="status-symbol">⌘</div><div class="status-copy"><strong>Build the system your team will use</strong><p>Use Build to configure Records, group them into installable Modules, and define business rules. Switch to View to see the working experience.</p></div><span class="api-pill" :class="`is-${apiState}`"><i></i>{{ apiState === 'connected' ? 'API connected' : 'API offline' }}</span></div>
    <div class="section-heading"><div><h2>Workspace building blocks</h2><p>Define the data layer, then compose reusable capabilities</p></div></div>
    <div class="foundation-grid">
      <button class="foundation-card" @click="emit('navigate', 'records')"><div class="card-top"><span class="card-icon">▤</span><span class="card-tag">DATA MODEL</span></div><h3>Records</h3><p>Define a Record's fields and MariaDB table schema; trusted behavior is implemented in Go code.</p><div class="card-footer"><span class="muted-label">{{ records.length }} definitions</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'modules')"><div class="card-top"><span class="card-icon">▦</span><span class="card-tag">COMPOSED CAPABILITIES</span></div><h3>Modules</h3><p>Connect Records through relationships and package their workflows into reusable capabilities.</p><div class="card-footer"><span class="muted-label">Compose from Records</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'rules')"><div class="card-top"><span class="card-icon">⌘</span><span class="card-tag">CONFIGURABLE LOGIC</span></div><h3>Rules</h3><p>Use Go for trusted business logic; Starlark is planned for configurable conditions and actions.</p><div class="card-footer"><span class="muted-label">Go core · Starlark planned</span><span class="arrow">→</span></div></button>
    </div>
    <div class="lower-grid"><section class="panel"><div class="panel-heading"><div><h2>Records</h2><p>Each Record definition maps to a MariaDB table</p></div><button class="text-button" @click="emit('navigate', 'records')">Open builder →</button></div><button v-for="record in records" :key="record.name" class="list-row" @click="emit('navigate', 'records')"><span class="list-icon">▤</span><span class="row-main"><strong>{{ record.label }}</strong><small>{{ record.name }} · {{ record.fields.length }} fields</small></span><span class="row-meta">Definition</span><span class="arrow">→</span></button><div v-if="records.length === 0" class="empty-state">No Records yet. Create one to define your first table.</div></section>
      <section class="panel"><div class="panel-heading"><div><h2>Instance status</h2><p>Connection to this Athena server</p></div><span class="live-indicator"><i></i>{{ apiState === 'connected' ? 'LIVE' : 'OFFLINE' }}</span></div><div class="server-details"><div><span>Frontend</span><strong>Vue 3 · TypeScript · Vite</strong></div><div><span>API</span><strong>Go · ConnectRPC</strong></div><div><span>Server</span><strong>Same origin · port 3000</strong></div><div><span>MariaDB</span><strong>{{ databaseState }}</strong></div></div><p class="database-hint">Set <code>ATHENA_DATABASE_URL</code> to connect MariaDB.</p></section></div>
  </section>
</template>
