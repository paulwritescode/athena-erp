<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { RecordType, Page } from '../types'
import ActionButton from '@/components/ActionButton.vue'
import { ruleClient } from '../api'
defineProps<{ recordTypes: RecordType[]; databaseState: string; apiState: string }>()
const emit = defineEmits<{ navigate: [page: Page]; createRecordType: [] }>()
// Live Rule count for the Rules card. Stays null when the list cannot be
// read (for example without the Rules permission); the card then omits it.
const ruleCount = ref<number | null>(null)
onMounted(async () => {
  try {
    ruleCount.value = (await ruleClient.listRules({})).rules.length
  } catch {
    ruleCount.value = null
  }
})
</script>

<template>
  <section class="page-content">
    <div class="welcome-row"><div><h1>Overview</h1><p class="lead">Define Record Types, group them into Packs and add Rules.</p></div><ActionButton kind="primary" label="New Record Type" icon="plus" @press="emit('createRecordType')" /></div>
    <p v-if="apiState === 'error' || databaseState !== 'Connected'" class="notice-row">{{ apiState === 'error' ? 'The Athena server is unavailable.' : `Database: ${databaseState}.` }}</p>
    <div class="foundation-grid">
      <button class="foundation-card" @click="emit('navigate', 'record-types')"><h2>Record Types</h2><p>Fields that define one kind of record.</p><div class="card-footer"><span class="muted-label">{{ recordTypes.length }} {{ recordTypes.length === 1 ? 'Record Type' : 'Record Types' }}</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'packs')"><h2>Packs</h2><p>Record Types and relationships packaged as an installable Pack.</p><div class="card-footer"><span class="muted-label">Later beta</span><span class="arrow">→</span></div></button>
      <button class="foundation-card" @click="emit('navigate', 'rules')"><h2>Rules</h2><p>Configurable conditions and actions in Starlark.</p><div class="card-footer"><span class="muted-label">{{ ruleCount === null ? '' : ruleCount === 0 ? 'No rules yet' : `${ruleCount} ${ruleCount === 1 ? 'rule' : 'rules'}` }}</span><span class="arrow">→</span></div></button>
    </div>
    <section class="panel"><div class="panel-heading"><h2>Record Types</h2><button class="text-button" @click="emit('navigate', 'record-types')">Open →</button></div><button v-for="recordType in recordTypes" :key="recordType.name" class="list-row" @click="emit('navigate', 'record-types')"><span class="row-main"><strong>{{ recordType.label }}</strong><small>{{ recordType.fields.length }} {{ recordType.fields.length === 1 ? 'field' : 'fields' }}</small></span><span class="arrow">→</span></button><div v-if="recordTypes.length === 0" class="empty-state">No Record Types yet.</div></section>
  </section>
</template>
