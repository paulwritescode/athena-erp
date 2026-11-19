<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue'
import type { Field, RecordDefinition } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog'
const props = defineProps<{ records: RecordDefinition[]; currentRecord: RecordDefinition; databaseState: string }>()
const emit = defineEmits<{ select: [name: string]; addRecord: []; addField: []; rename: [from: string, to: string]; delete: [record: RecordDefinition]; save: [complete: (saved: boolean) => void]; preview: [] }>()
const search = ref('')
const newFields = reactive(new Set<Field>())
const manuallyNamedFields = new WeakSet<Field>()
const fieldKeys = new WeakMap<Field, number>()
let nextFieldKey = 0
const filteredRecords = computed(() => props.records.filter((item) => `${item.label} ${item.name}`.toLowerCase().includes(search.value.toLowerCase())))

function fieldKey(field: Field) {
  let key = fieldKeys.get(field)
  if (key === undefined) {
    key = nextFieldKey++
    fieldKeys.set(field, key)
  }
  return key
}

function snakeKey(text: string, fallback: string, taken: Set<string>, reserved: string[]) {
  let base = text.normalize('NFKD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
  if (!base) base = fallback
  if (reserved.includes(base)) base = `${fallback}_${base}`
  if (/^[0-9]/.test(base)) base = `${fallback}_${base}`
  let candidate = base
  let suffix = 2
  while (taken.has(candidate)) candidate = `${base}_${suffix++}`
  return candidate
}
function fieldNameFromLabel(label: string, field: Field) {
  const taken = new Set(props.currentRecord.fields.filter((item) => item !== field).map((item) => item.name.toLowerCase()))
  return snakeKey(label, 'field', taken, ['id', 'revision', 'created_at', 'updated_at'])
}
const manuallyNamedRecords = new Set<string>()
const recordIsSaved = computed(() => props.currentRecord.revision > 0)
function recordKeyFrom(text: string) {
  const taken = new Set(props.records.filter((item) => item !== props.currentRecord).map((item) => item.name.toLowerCase()))
  return snakeKey(text, 'record', taken, [])
}
function renameRecord(name: string) {
  const from = props.currentRecord.name
  if (name === from) return
  if (manuallyNamedRecords.delete(from)) manuallyNamedRecords.add(name)
  emit('rename', from, name)
}
function updateRecordLabel(label: string | number) {
  const wasAuto = !manuallyNamedRecords.has(props.currentRecord.name)
  props.currentRecord.label = String(label)
  if (!recordIsSaved.value && wasAuto) renameRecord(recordKeyFrom(props.currentRecord.label))
}
function typeRecordName(event: Event) {
  const from = props.currentRecord.name
  const value = (event.target as HTMLInputElement).value
  if (!value) return
  manuallyNamedRecords.add(from)
  renameRecord(value)
}
function normalizeRecordName() {
  if (!recordIsSaved.value) renameRecord(recordKeyFrom(props.currentRecord.name))
}
function updateFieldLabel(field: Field, label: string | number) {
  field.label = String(label)
  if (newFields.has(field) && !manuallyNamedFields.has(field)) field.name = fieldNameFromLabel(field.label, field)
}

function updateFieldName(field: Field, name: string | number) {
  field.name = String(name)
  manuallyNamedFields.add(field)
}

function normalizeNewFieldName(field: Field) {
  if (newFields.has(field)) field.name = fieldNameFromLabel(field.name, field)
}

function addField() {
  emit('addField')
  void nextTick(() => {
    const field = props.currentRecord.fields.at(-1)
    if (field) newFields.add(field)
  })
}

function save() {
  for (const field of props.currentRecord.fields) normalizeNewFieldName(field)
  emit('save', (saved) => {
    if (saved) for (const field of props.currentRecord.fields) newFields.delete(field)
  })
}

function isNewField(field: Field) {
  return newFields.has(field)
}
</script>

<template>
  <section class="page-content builder-page">
    <div class="page-title-row"><div><h1>Records</h1><p class="lead">Each Record defines the fields of one table.</p></div><Button @click="emit('addRecord')">New Record</Button></div>
    <div v-if="records.length === 0" class="empty-builder"><h2>No Records yet</h2><p>Create a Record, add its fields and save it to start adding entries.</p><Button @click="emit('addRecord')">Create your first Record</Button></div>
    <div v-else class="records-layout">
      <aside class="records-list">
        <Input v-if="records.length > 6" v-model="search" placeholder="Find a Record" aria-label="Find a Record" />
        <button v-for="record in filteredRecords" :key="record.name" class="records-list-item" :class="{ chosen: currentRecord.name === record.name }" @click="emit('select', record.name)">{{ record.label || 'Untitled' }}</button>
        <p v-if="filteredRecords.length === 0" class="empty-state">No matching Records.</p>
      </aside>
      <div class="record-editor">
        <div class="record-editor-head">
          <Input :model-value="currentRecord.label" class="record-title-input" placeholder="Record name" aria-label="Record label" @update:model-value="updateRecordLabel" />
          <span class="muted-label">{{ recordIsSaved ? 'Saved' : 'Not saved' }}</span>
          <div class="toolbar-actions">
            <AlertDialog>
              <AlertDialogTrigger as-child><Button variant="ghost" size="sm">Delete</Button></AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{{ recordIsSaved ? `Delete ${currentRecord.label}?` : `Discard ${currentRecord.label}?` }}</AlertDialogTitle>
                  <AlertDialogDescription>{{ recordIsSaved ? 'Its table and every entry in it are deleted. This cannot be undone.' : 'This Record has not been saved, so nothing is stored yet.' }}</AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction variant="destructive" @click="emit('delete', currentRecord)">{{ recordIsSaved ? 'Delete Record' : 'Discard' }}</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
            <Button variant="outline" size="sm" :disabled="!recordIsSaved" :title="recordIsSaved ? '' : 'Save the Record first'" @click="emit('preview')">Entries</Button><Button size="sm" @click="save">Save</Button></div>
        </div>
        <div class="record-meta">
          <label>Key<Input :model-value="currentRecord.name" :readonly="recordIsSaved" aria-label="Record name" @input="typeRecordName" @blur="normalizeRecordName" /><small>{{ recordIsSaved ? 'Fixed after the first save.' : 'Follows the name until you edit it.' }}</small></label>
          <label>Description<Input v-model="currentRecord.description" placeholder="Optional" aria-label="Description" /></label>
        </div>
        <div class="field-table"><div class="field-head"><span>Label</span><span>Key</span><span>Type</span><span>Required</span><span></span></div><div v-for="(field, index) in currentRecord.fields" :key="fieldKey(field)" class="field-row"><span><Input :model-value="field.label" :aria-label="`Field ${index + 1} label`" @update:model-value="updateFieldLabel(field, $event)" /></span><span><Input :model-value="field.name" :readonly="!isNewField(field)" :aria-label="`Field ${index + 1} database name`" @update:model-value="updateFieldName(field, $event)" @blur="normalizeNewFieldName(field)" /></span><span><select v-model="field.type" aria-label="Field type"><option>Text</option><option>Long text</option><option>Email</option><option>Phone</option><option>Number</option><option>Currency</option><option>Date</option><option>Select</option><option>Link</option><option>Checkbox</option></select></span><span><input v-model="field.required" type="checkbox" :aria-label="`${field.label} required`" /></span><span><button class="icon-action" :aria-label="`Remove ${field.label}`" @click="currentRecord.fields.splice(index, 1)">×</button></span><div v-if="field.type === 'Link'" class="relationship-editor"><label>Links to<select v-model="field.target_record"><option :value="undefined" disabled>Select a Record</option><option v-for="target in records.filter((item) => item.name !== currentRecord.name)" :key="target.name" :value="target.name">{{ target.label }}</option></select></label></div></div></div>
        <button class="add-field" @click="addField">+ Add field</button>
        <p v-if="databaseState !== 'Connected'" class="muted-label">The database is not connected. Changes stay in this browser session.</p>
      </div>
    </div>
  </section>
</template>
