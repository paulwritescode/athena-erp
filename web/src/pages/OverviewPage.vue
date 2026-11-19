<script setup lang="ts">
import type { RecordDefinition, Page } from '../types'
import { Button } from '@/components/ui/button'
defineProps<{ records: RecordDefinition[]; databaseState: string; apiState: string }>()
const emit = defineEmits<{ navigate: [page: Page]; createRecord: [] }>()
</script>

<template>
  <section class="page-content">
    <div class="welcome-row"><div><h1>Workspace</h1><p class="lead">Define Records, group them into Modules and add Rules.</p></div><Button @click="emit('createRecord')">New Record</Button></div>
    <p v-if="apiState === 'error' || databaseState !== 'Connected'" class="notice-row">{{ apiState === 'error' ? 'The Athena server is unavailable.' : `Database: ${databaseState}.` }}</p>
    <div class="foundation-grid">
      <button class="foundation-card" @click="emit('navigate', 'records')"><h3>Records</h3><p>Fields that define a table in the project database.</p><div class="card-footer"><span class="muted-label">{{ records.length }} {{ records.length === 1 ? 'Record' : 'Records' }}</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'modules')"><h3>Modules</h3><p>Records and relationships packaged as a reusable capability.</p><div class="card-footer"><span class="muted-label">Planned</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'rules')"><h3>Rules</h3><p>Configurable conditions and actions in Starlark.</p><div class="card-footer"><span class="muted-label">Planned</span><span class="arrow">→</span></div></button>
    </div>
    <section class="panel"><div class="panel-heading"><h2>Records</h2><button class="text-button" @click="emit('navigate', 'records')">Open →</button></div><button v-for="record in records" :key="record.name" class="list-row" @click="emit('navigate', 'records')"><span class="row-main"><strong>{{ record.label }}</strong><small>{{ record.fields.length }} {{ record.fields.length === 1 ? 'field' : 'fields' }}</small></span><span class="arrow">→</span></button><div v-if="records.length === 0" class="empty-state">No Records yet.</div></section>
  </section>
</template>
