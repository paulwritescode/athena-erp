<script setup lang="ts">
import { computed, ref } from 'vue'
import type { RecordDefinition } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
const props = defineProps<{ records: RecordDefinition[]; currentRecord: RecordDefinition; databaseState: string }>()
const emit = defineEmits<{ select: [name: string]; addRecord: []; addField: []; save: []; preview: [] }>()
const showNewRecord = ref(false)
const search = ref('')
const filteredRecords = computed(() => props.records.filter((item) => `${item.label} ${item.name}`.toLowerCase().includes(search.value.toLowerCase())))
</script>

<template>
  <section class="page-content builder-page">
    <div class="page-title-row"><div><div class="eyebrow">BUILD · DATA MODEL</div><h1>Records</h1><p class="lead">Configure the fields that define a MariaDB record table.</p></div><Button @click="showNewRecord = !showNewRecord">＋ New Record</Button></div>
    <div v-if="showNewRecord" class="notice-row">Create a new Record definition?<Button size="sm" @click="emit('addRecord'); showNewRecord = false">Create Record</Button><Button size="sm" variant="ghost" @click="showNewRecord = false">Cancel</Button></div>
    <div v-if="records.length === 0" class="empty-builder"><div class="card-icon">▤</div><h2>No Records yet</h2><p>Create a Record to define its fields, generated Go behavior hooks, and database table.</p><Button @click="emit('addRecord')">Create your first Record</Button></div>
    <div v-else class="builder-layout">
      <aside class="builder-list"><div class="builder-list-heading"><strong>RECORD DEFINITIONS</strong><span>{{ records.length }}</span></div><label class="search-box"><span>⌕</span><Input v-model="search" placeholder="Find a Record" /></label><button v-for="record in filteredRecords" :key="record.name" class="record-choice" :class="{ chosen: currentRecord.name === record.name }" @click="emit('select', record.name)"><span class="list-icon">▤</span><span><strong>{{ record.label }}</strong><small>{{ record.name }}</small></span></button><div v-if="filteredRecords.length === 0" class="empty-state">No matching Records.</div></aside>
      <div class="editor-column"><div class="editor-toolbar"><div><span class="breadcrumb-label">Records /</span><strong>{{ currentRecord.label }}</strong><span class="status-chip">Draft</span></div><div class="toolbar-actions"><Button variant="outline" size="sm" @click="emit('preview')">Preview</Button><Button size="sm" @click="emit('save')">Save</Button></div></div>
        <div class="editor-card"><div class="record-heading"><div class="record-symbol">▤</div><div><Input v-model="currentRecord.label" class="title-input" aria-label="Record name"/><p>{{ currentRecord.description }}</p></div></div>
          <div class="tab-strip"><button class="active">Fields</button><button>Rules <span class="planned-tag">PLANNED</span></button><button>Module</button><button>Permissions <span class="planned-tag">PLANNED</span></button></div>
          <div class="field-table"><div class="field-head"><span>LABEL</span><span>FIELD NAME</span><span>TYPE</span><span>REQUIRED</span><span></span></div><div v-for="(field, index) in currentRecord.fields" :key="field.name + index" class="field-row"><span><Input v-model="field.label" :aria-label="`Field ${index + 1} label`" /></span><span><Input v-model="field.name" :aria-label="`Field ${index + 1} database name`" /></span><span><select v-model="field.type" aria-label="Field type"><option>Text</option><option>Long text</option><option>Email</option><option>Phone</option><option>Number</option><option>Currency</option><option>Date</option><option>Select</option><option>Link</option><option>Checkbox</option></select></span><span><input v-model="field.required" type="checkbox" :aria-label="`${field.label} required`" /></span><span><button class="icon-action" :aria-label="`Remove ${field.label}`" @click="currentRecord.fields.splice(index, 1)">×</button></span><div v-if="field.type === 'Link'" class="relationship-editor"><label>References Record<select v-model="field.target_record"><option value="">Select target</option><option v-for="target in records.filter((item) => item.name !== currentRecord.name)" :key="target.name" :value="target.name">{{ target.label }}</option></select></label></div></div></div>
          <Button variant="ghost" size="sm" class="add-field" @click="emit('addField')">＋ Add field</Button><div class="editor-footnote">{{ databaseState === 'Connected' ? 'Changes can be saved to MariaDB.' : 'Changes are in browser memory until MariaDB is connected.' }}</div>
        </div>
      </div>
      <aside class="inspector"><div class="inspector-title">RECORD SETTINGS</div><label>Record name<input :value="currentRecord.name" readonly /></label><label>Database table<input :value="`athena_record_${currentRecord.name.toLowerCase()}`" readonly /></label><label>Description<textarea v-model="currentRecord.description" rows="3" /></label><div class="inspector-note"><strong>Record definition</strong><p>A Record defines one MariaDB table. Modules will connect Records through explicit relationships.</p></div></aside>
    </div>
  </section>
</template>
