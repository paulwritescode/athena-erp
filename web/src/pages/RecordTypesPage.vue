<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue'
import type { Field, Kind, RecordType } from '../types'
import { validateNamingSeries } from '../fieldRules'
import FieldSettingsDialog from '../components/FieldSettingsDialog.vue'
import ActionButton from '@/components/ActionButton.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
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
  { value: 'document', label: 'Document', hint: 'A business event with a class. Switch on Submittable to give it a draft, submitted and cancelled lifecycle.' },
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
  else { delete recordType.class; delete recordType.submittable; delete recordType.amendable; delete recordType.naming_series; delete recordType.gap_free; delete recordType.allow_manual_name }
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

// Work documents have no submit step, so the server refuses the combination.
function chooseClass() {
  if (props.currentRecordType.class === 'work') setSubmittable(false)
}
function setSubmittable(value: boolean) {
  const recordType = props.currentRecordType
  if (value) recordType.submittable = true
  else { delete recordType.submittable; delete recordType.amendable; delete recordType.naming_series; delete recordType.gap_free; delete recordType.allow_manual_name }
}
function setNamingSeries(value: string) {
  const recordType = props.currentRecordType
  if (value.trim()) recordType.naming_series = value
  else { delete recordType.naming_series; delete recordType.gap_free }
}
function setFlag(key: 'gap_free' | 'allow_manual_name', value: boolean) {
  if (value) props.currentRecordType[key] = true
  else delete props.currentRecordType[key]
}
function setLabelField(value: string) {
  if (value) props.currentRecordType.label_field = value
  else delete props.currentRecordType.label_field
}
// Fields that can label a Record in a picker (internal/server/naming.go).
const labelFieldChoices = computed(() => props.currentRecordType.fields.filter((field) => ['Text', 'Long text', 'Email', 'Phone', 'Select', 'Enum', 'Date'].includes(field.type) && !isNewField(field)))
const namingProblem = computed(() => validateNamingSeries(props.currentRecordType.naming_series ?? ''))
function setAmendable(value: boolean) {
  if (value) props.currentRecordType.amendable = true
  else delete props.currentRecordType.amendable
}
const settingsField = ref<Field | null>(null)
const confirmingTypeDelete = ref(false)
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
    <div class="page-title-row"><div><h1>Record Types</h1><p class="lead">Each Record Type defines the fields of one kind of record.</p></div><ActionButton kind="primary" label="New Record Type" icon="plus" @press="emit('addRecordType')" /></div>
    <div v-if="recordTypes.length === 0" class="empty-builder"><h2>No Record Types yet</h2><p>Create a Record Type, add its fields and save it to start adding records.</p><ActionButton kind="primary" label="Create your first Record Type" icon="plus" @press="emit('addRecordType')" /></div>
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
            <ActionButton kind="secondary" density="row" label="View records" icon="eye" :disabled-reason="recordTypeIsSaved ? null : 'Save the Record Type first.'" @press="emit('preview')" />
            <ActionButton kind="primary" density="row" label="Save" icon="save" @press="save" />
            <ActionButton kind="destructive" density="row" label="Delete" icon="trash-2" @press="confirmingTypeDelete = true" />
            <ConfirmDialog
              v-model:open="confirmingTypeDelete"
              :title="recordTypeIsSaved ? `Delete ${currentRecordType.label}?` : `Discard ${currentRecordType.label}?`"
              :body="recordTypeIsSaved ? 'Its table and every record in it are deleted. This cannot be undone.' : 'This Record Type has not been saved, so nothing is stored yet.'"
              :confirm-label="recordTypeIsSaved ? 'Delete Record Type' : 'Discard'"
              :confirm-icon="recordTypeIsSaved ? 'trash-2' : 'x'"
              :danger="recordTypeIsSaved"
              @confirm="confirmingTypeDelete = false; emit('delete', currentRecordType)"
            />
          </div>
        </div>
        <div class="record-meta">
          <label>Key<Input :model-value="currentRecordType.name" :readonly="recordTypeIsSaved" aria-label="Record Type name" @input="typeRecordTypeName" @blur="normalizeRecordTypeName" /><small>{{ recordTypeIsSaved ? 'Fixed after the first save.' : 'Follows the name until you edit it.' }}</small></label>
          <label>Description<Input v-model="currentRecordType.description" placeholder="Optional" aria-label="Description" /></label>
        </div>
        <div class="record-meta record-kind">
          <label>Kind<select :value="currentRecordType.kind" :disabled="recordTypeIsSaved" aria-label="Record Type kind" @change="chooseKind(($event.target as HTMLSelectElement).value as Kind)"><option v-for="kind in kinds" :key="kind.value" :value="kind.value">{{ kind.label }}</option></select><small>{{ recordTypeIsSaved ? 'Fixed after the first save. ' : '' }}{{ kindHint }}</small></label>
          <label v-if="currentRecordType.kind === 'document'">Class<select v-model="currentRecordType.class" aria-label="Document class" @change="chooseClass"><option v-for="item in documentClasses" :key="item[0]" :value="item[0]">{{ item[1] }}</option></select><small>Posting documents will write to ledgers; work and batch documents track work.</small></label>
          <label v-if="currentRecordType.kind === 'document' && currentRecordType.class !== 'work'" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.submittable ?? false" aria-label="Submittable" @update:model-value="setSubmittable($event === true)" />Submittable</span><small>Records move from draft to submitted to cancelled. Submitted records stay fixed, apart from fields set to allow editing.</small></label>
          <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.amendable ?? false" aria-label="Amendable" @update:model-value="setAmendable($event === true)" />Amendable</span><small>A cancelled record can be amended into a new draft that points back to it.</small></label>
          <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable" :class="{ 'is-invalid': namingProblem }">Naming series<Input :model-value="currentRecordType.naming_series ?? ''" placeholder="INV-{yyyy}-{#####}" aria-label="Naming series" :aria-invalid="namingProblem ? 'true' : undefined" @update:model-value="setNamingSeries(String($event))" /><small>{{ namingProblem || 'Numbers each document when it is submitted. Tokens: {yyyy}, {yy}, {mm} and one counter such as {#####}. Empty keeps the Record ID as the name.' }}</small></label>
          <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable && currentRecordType.naming_series" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.gap_free ?? false" aria-label="Gap-free numbering" @update:model-value="setFlag('gap_free', $event === true)" />Gap-free numbering</span><small>A submit that fails returns its number, so the series has no gaps. Submits take turns. Without it numbers can skip.</small></label>
          <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.allow_manual_name ?? false" aria-label="Allow manual names" @update:model-value="setFlag('allow_manual_name', $event === true)" />Allow manual names</span><small>A new document can take a name typed by hand, for example when moving old documents in. It must be unused and is logged.</small></label>
          <label v-if="recordTypeIsSaved">Label field<select :value="currentRecordType.label_field ?? ''" aria-label="Label field" @change="setLabelField(($event.target as HTMLSelectElement).value)"><option value="">Automatic</option><option v-for="field in labelFieldChoices" :key="field.name" :value="field.name">{{ field.label }}</option></select><small>The field that names a Record when it is chosen from a list. Automatic uses the record name, then the first required text field.</small></label>
          <label v-if="currentRecordType.kind === 'child'">Belongs to<select v-model="currentRecordType.parent_record_type" :disabled="recordTypeIsSaved" aria-label="Parent Record Type"><option :value="undefined" disabled>Select a saved Record Type</option><option v-for="item in parentChoices" :key="item.name" :value="item.name">{{ item.label }}</option></select><small>{{ recordTypeIsSaved ? 'Fixed after the first save.' : 'Save the owning Record Type first.' }}</small></label>
        </div>
        <div class="field-table"><div class="field-head"><span>Label</span><span>Key</span><span>Type</span><span>Required</span><span></span></div><div v-for="(field, index) in currentRecordType.fields" :key="fieldKey(field)" class="field-row"><span><Input :model-value="field.label" :aria-label="`Field ${index + 1} label`" @update:model-value="updateFieldLabel(field, $event)" /></span><span><Input :model-value="field.name" :readonly="!isNewField(field)" :aria-label="`Field ${index + 1} database name`" @update:model-value="updateFieldName(field, $event)" @blur="normalizeNewFieldName(field)" /></span><span><select v-model="field.type" aria-label="Field type" @change="changeType(field)"><option v-for="type in fieldTypes" :key="type">{{ type }}</option></select></span><span><input v-model="field.required" type="checkbox" :aria-label="`${field.label} required`" /></span><span class="field-actions"><ActionButton kind="quiet" density="inline" icon-only :label="`Settings for ${field.label}`" icon="sliders-horizontal" @press="settingsField = field" /><ActionButton kind="quiet" density="inline" icon-only :label="`Remove ${field.label}`" icon="x" @press="currentRecordType.fields.splice(index, 1)" /></span><div v-if="field.type === 'Link'" class="relationship-editor"><label>Links to<select v-model="field.target_record_type"><option :value="undefined" disabled>Select a Record Type</option><option v-for="target in recordTypes.filter((item) => item.name !== currentRecordType.name)" :key="target.name" :value="target.name">{{ target.label }}</option></select></label></div></div></div>
        <ActionButton kind="quiet" label="Add field" icon="plus" @press="addField" />
        <p v-if="databaseState !== 'Connected'" class="muted-label">The database is not connected. Changes stay in this browser session.</p>
      </div>
    </div>
    <FieldSettingsDialog :field="settingsField" :lifecycle="(currentRecordType.kind === 'document' && currentRecordType.submittable === true) || currentRecordType.kind === 'child'" @close="settingsField = null" />
  </section>
</template>
