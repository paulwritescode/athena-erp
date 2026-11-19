<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue'
import type { Field, Kind, RecordType } from '../types'
import FieldSettingsDialog from '../components/FieldSettingsDialog.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SlidersHorizontalIcon } from '@lucide/vue'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog'
const props = defineProps<{ recordTypes: RecordType[]; currentRecordType: RecordType; databaseState: string }>()
const emit = defineEmits<{ select: [name: string]; addRecordType: []; addField: []; rename: [from: string, to: string]; delete: [recordType: RecordType]; save: [complete: (saved: boolean) => void]; preview: [] }>()
const search = ref('')
const newFields = reactive(new Set<Field>())
const manuallyNamedFields = new WeakSet<Field>()
const fieldKeys = new WeakMap<Field, number>()
let nextFieldKey = 0
const filteredRecordTypes = computed(() => props.recordTypes.filter((item) => `${item.label} ${item.name}`.toLowerCase().includes(search.value.toLowerCase())))

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
  const taken = new Set(props.currentRecordType.fields.filter((item) => item !== field).map((item) => item.name.toLowerCase()))
  return snakeKey(label, 'field', taken, ['id', 'revision', 'created_at', 'updated_at', 'created_by', 'modified_by', 'owner', 'deleted_at', 'def_version', 'request_id', 'parent_id', 'idx'])
}
const manuallyNamedRecordTypes = new Set<string>()
const recordTypeIsSaved = computed(() => props.currentRecordType.revision > 0)
function recordTypeKeyFrom(text: string) {
  const taken = new Set(props.recordTypes.filter((item) => item !== props.currentRecordType).map((item) => item.name.toLowerCase()))
  return snakeKey(text, 'record_type', taken, [])
}
function renameRecordType(name: string) {
  const from = props.currentRecordType.name
  if (name === from) return
  if (manuallyNamedRecordTypes.delete(from)) manuallyNamedRecordTypes.add(name)
  emit('rename', from, name)
}
function updateRecordTypeLabel(label: string | number) {
  const wasAuto = !manuallyNamedRecordTypes.has(props.currentRecordType.name)
  props.currentRecordType.label = String(label)
  if (!recordTypeIsSaved.value && wasAuto) renameRecordType(recordTypeKeyFrom(props.currentRecordType.label))
}
function typeRecordTypeName(event: Event) {
  const from = props.currentRecordType.name
  const value = (event.target as HTMLInputElement).value
  if (!value) return
  manuallyNamedRecordTypes.add(from)
  renameRecordType(value)
}
function normalizeRecordTypeName() {
  if (!recordTypeIsSaved.value) renameRecordType(recordTypeKeyFrom(props.currentRecordType.name))
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
    const field = props.currentRecordType.fields.at(-1)
    if (field) newFields.add(field)
  })
}

const kinds: { value: Kind; label: string; hint: string }[] = [
  { value: 'master', label: 'Master', hint: 'A long-lived thing, like a customer or an item. Deleting hides it and keeps the row.' },
  { value: 'document', label: 'Document', hint: 'A business event with a class. Records of this kind are drafts for now.' },
  { value: 'child', label: 'Child', hint: 'A line that belongs to one record of another Record Type.' },
  { value: 'setting', label: 'Setting', hint: 'One record that holds configuration. It is edited, never deleted.' },
]
const documentClasses = [['posting', 'Posting'], ['non_posting', 'Non-posting'], ['work', 'Work'], ['batch', 'Batch']]
// A child belongs to a master or a document that has been saved.
const parentChoices = computed(() => props.recordTypes.filter((item) => item.revision > 0 && (item.kind === 'master' || item.kind === 'document') && item.name !== props.currentRecordType.name))
const kindHint = computed(() => kinds.find((item) => item.value === props.currentRecordType.kind)?.hint ?? '')

function chooseKind(kind: Kind) {
  const recordType = props.currentRecordType
  recordType.kind = kind
  if (kind === 'document') recordType.class = recordType.class ?? 'non_posting'
  else delete recordType.class
  if (kind === 'child') recordType.parent_record_type = recordType.parent_record_type ?? parentChoices.value[0]?.name
  else delete recordType.parent_record_type
}

// Attributes belong to a type, so switching type drops the ones that no longer
// apply and fills in the settings Money and Quantity cannot save without.
function changeType(field: Field) {
  const keep = new Set(['name', 'label', 'type', 'required', 'unique', 'index', 'read_only', 'set_once', 'no_copy'])
  for (const key of Object.keys(field)) if (!keep.has(key)) delete (field as Record<string, unknown>)[key]
  if (field.type === 'Money') { field.currency = 'USD'; field.scale = 2 }
  if (field.type === 'Quantity') field.uom = 'unit'
  if (field.type === 'Enum') field.values = [{ value: 'option_1', label: 'Option 1' }]
  if (field.type === 'Long text' || field.type === 'Checkbox') { delete field.unique; delete field.index }
}

const settingsField = ref<Field | null>(null)
const fieldTypes = ['Text', 'Long text', 'Email', 'Phone', 'Number', 'Currency', 'Money', 'Quantity', 'Date', 'Select', 'Enum', 'Link', 'Checkbox']

function save() {
  for (const field of props.currentRecordType.fields) normalizeNewFieldName(field)
  emit('save', (saved) => {
    if (saved) for (const field of props.currentRecordType.fields) newFields.delete(field)
  })
}

function isNewField(field: Field) {
  return newFields.has(field)
}
</script>

<template>
  <section class="page-content builder-page">
    <div class="page-title-row"><div><h1>Record Types</h1><p class="lead">Each Record Type defines the fields of one table.</p></div><Button @click="emit('addRecordType')">New Record Type</Button></div>
    <div v-if="recordTypes.length === 0" class="empty-builder"><h2>No Record Types yet</h2><p>Create a Record Type, add its fields and save it to start adding records.</p><Button @click="emit('addRecordType')">Create your first Record Type</Button></div>
    <div v-else class="records-layout">
      <aside class="records-list">
        <Input v-if="recordTypes.length > 6" v-model="search" placeholder="Find a Record Type" aria-label="Find a Record Type" />
        <button v-for="recordType in filteredRecordTypes" :key="recordType.name" class="records-list-item" :class="{ chosen: currentRecordType.name === recordType.name }" @click="emit('select', recordType.name)">{{ recordType.label || 'Untitled' }}</button>
        <p v-if="filteredRecordTypes.length === 0" class="empty-state">No matching Record Types.</p>
      </aside>
      <div class="record-editor">
        <div class="record-editor-head">
          <Input :model-value="currentRecordType.label" class="record-title-input" placeholder="Record Type name" aria-label="Record Type label" @update:model-value="updateRecordTypeLabel" />
          <span class="muted-label">{{ recordTypeIsSaved ? 'Saved' : 'Not saved' }}</span>
          <div class="toolbar-actions">
            <AlertDialog>
              <AlertDialogTrigger as-child><Button variant="ghost" size="sm">Delete</Button></AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{{ recordTypeIsSaved ? `Delete ${currentRecordType.label}?` : `Discard ${currentRecordType.label}?` }}</AlertDialogTitle>
                  <AlertDialogDescription>{{ recordTypeIsSaved ? 'Its table and every record in it are deleted. This cannot be undone.' : 'This Record Type has not been saved, so nothing is stored yet.' }}</AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction variant="destructive" @click="emit('delete', currentRecordType)">{{ recordTypeIsSaved ? 'Delete Record Type' : 'Discard' }}</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
            <Button variant="outline" size="sm" :disabled="!recordTypeIsSaved" :title="recordTypeIsSaved ? '' : 'Save the Record Type first'" @click="emit('preview')">Record Types</Button><Button size="sm" @click="save">Save</Button></div>
        </div>
        <div class="record-meta">
          <label>Key<Input :model-value="currentRecordType.name" :readonly="recordTypeIsSaved" aria-label="Record Type name" @input="typeRecordTypeName" @blur="normalizeRecordTypeName" /><small>{{ recordTypeIsSaved ? 'Fixed after the first save.' : 'Follows the name until you edit it.' }}</small></label>
          <label>Description<Input v-model="currentRecordType.description" placeholder="Optional" aria-label="Description" /></label>
        </div>
        <div class="record-meta record-kind">
          <label>Kind<select :value="currentRecordType.kind" :disabled="recordTypeIsSaved" aria-label="Record Type kind" @change="chooseKind(($event.target as HTMLSelectElement).value as Kind)"><option v-for="kind in kinds" :key="kind.value" :value="kind.value">{{ kind.label }}</option></select><small>{{ recordTypeIsSaved ? 'Fixed after the first save. ' : '' }}{{ kindHint }}</small></label>
          <label v-if="currentRecordType.kind === 'document'">Class<select v-model="currentRecordType.class" aria-label="Document class"><option v-for="item in documentClasses" :key="item[0]" :value="item[0]">{{ item[1] }}</option></select><small>Posting documents will write to ledgers; work and batch documents track work.</small></label>
          <label v-if="currentRecordType.kind === 'child'">Belongs to<select v-model="currentRecordType.parent_record_type" :disabled="recordTypeIsSaved" aria-label="Parent Record Type"><option :value="undefined" disabled>Select a saved Record Type</option><option v-for="item in parentChoices" :key="item.name" :value="item.name">{{ item.label }}</option></select><small>{{ recordTypeIsSaved ? 'Fixed after the first save.' : 'Save the owning Record Type first.' }}</small></label>
        </div>
        <div class="field-table"><div class="field-head"><span>Label</span><span>Key</span><span>Type</span><span>Required</span><span></span></div><div v-for="(field, index) in currentRecordType.fields" :key="fieldKey(field)" class="field-row"><span><Input :model-value="field.label" :aria-label="`Field ${index + 1} label`" @update:model-value="updateFieldLabel(field, $event)" /></span><span><Input :model-value="field.name" :readonly="!isNewField(field)" :aria-label="`Field ${index + 1} database name`" @update:model-value="updateFieldName(field, $event)" @blur="normalizeNewFieldName(field)" /></span><span><select v-model="field.type" aria-label="Field type" @change="changeType(field)"><option v-for="type in fieldTypes" :key="type">{{ type }}</option></select></span><span><input v-model="field.required" type="checkbox" :aria-label="`${field.label} required`" /></span><span class="field-actions"><button class="icon-action" :aria-label="`Settings for ${field.label}`" title="Field settings" @click="settingsField = field"><SlidersHorizontalIcon /></button><button class="icon-action" :aria-label="`Remove ${field.label}`" @click="currentRecordType.fields.splice(index, 1)">×</button></span><div v-if="field.type === 'Link'" class="relationship-editor"><label>Links to<select v-model="field.target_record_type"><option :value="undefined" disabled>Select a Record Type</option><option v-for="target in recordTypes.filter((item) => item.name !== currentRecordType.name)" :key="target.name" :value="target.name">{{ target.label }}</option></select></label></div></div></div>
        <button class="add-field" @click="addField">+ Add field</button>
        <p v-if="databaseState !== 'Connected'" class="muted-label">The database is not connected. Changes stay in this browser session.</p>
      </div>
    </div>
    <FieldSettingsDialog :field="settingsField" @close="settingsField = null" />
  </section>
</template>
