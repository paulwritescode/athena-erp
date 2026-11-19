<script setup lang="ts">
import type { RecordType, Page } from '../types'
import { Button } from '@/components/ui/button'
defineProps<{ recordTypes: RecordType[]; databaseState: string; apiState: string }>()
const emit = defineEmits<{ navigate: [page: Page]; createRecordType: [] }>()
</script>

<template>
  <section class="page-content">
    <div class="welcome-row"><div><h1>Workspace</h1><p class="lead">Define Record Types, group them into Packs and add Rules.</p></div><Button @click="emit('createRecordType')">New Record Type</Button></div>
    <p v-if="apiState === 'error' || databaseState !== 'Connected'" class="notice-row">{{ apiState === 'error' ? 'The Athena server is unavailable.' : `Database: ${databaseState}.` }}</p>
    <div class="foundation-grid">
      <button class="foundation-card" @click="emit('navigate', 'record-types')"><h3>Record Types</h3><p>Fields that define a table in the Workspace database.</p><div class="card-footer"><span class="muted-label">{{ recordTypes.length }} {{ recordTypes.length === 1 ? 'Record Type' : 'Record Types' }}</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'packs')"><h3>Packs</h3><p>Record Types and relationships packaged as an installable Pack.</p><div class="card-footer"><span class="muted-label">Planned</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'rules')"><h3>Rules</h3><p>Configurable conditions and actions in Starlark.</p><div class="card-footer"><span class="muted-label">Planned</span><span class="arrow">→</span></div></button>
    </div>
    <section class="panel"><div class="panel-heading"><h2>Record Types</h2><button class="text-button" @click="emit('navigate', 'record-types')">Open →</button></div><button v-for="recordType in recordTypes" :key="recordType.name" class="list-row" @click="emit('navigate', 'record-types')"><span class="row-main"><strong>{{ recordType.label }}</strong><small>{{ recordType.fields.length }} {{ recordType.fields.length === 1 ? 'field' : 'fields' }}</small></span><span class="arrow">→</span></button><div v-if="recordTypes.length === 0" class="empty-state">No Record Types yet.</div></section>
  </section>
</template>
