<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { recordClient } from '../api'
import { keyMatchesRoute } from '../composables/useRecordTypes'
import { docstates, fieldEditable, isCalculated, validateField, validateManualName } from '../fieldRules'
import { gloss } from '../copy/glossary'
import { forgetLabels } from '../lookup'
import { formatWhen, formatWhenFull } from '../times'
import { FieldValueSchema, type ActivityEvent, type FieldChange, type FieldState, type Record as RecordMessage, type RecordEvent, type RecordVersion, type FieldValue } from '../gen/athena/v1/records_pb'
import type { Field, RecordType } from '../types'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import ActionButton from '@/components/ActionButton.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import BottomDrawer from '@/components/BottomDrawer.vue'
import PullToRefresh from '@/components/PullToRefresh.vue'
import RecordsTable from '../components/RecordsTable.vue'
import CheckboxField from '@/components/fields/CheckboxField.vue'
import DateField from '@/components/fields/DateField.vue'
import LinkField from '@/components/fields/LinkField.vue'
import LongTextField from '@/components/fields/LongTextField.vue'
import NumberField from '@/components/fields/NumberField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import TextField from '@/components/fields/TextField.vue'
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker'
import { CalendarPlus, Clock, Fingerprint, Hash, Layers, Pencil, Plus, Trash2, User } from '@/icons'
import { toast } from 'vue-sonner'

// recordType is null when no saved Record Type exists yet. recordId comes from the page
// URL: '' for the table, 'new' for the create form, or an record ID.
// recordTypeKey is the key the address names; writes send it, and a record
// type that does not match it never saves.
const props = defineProps<{ recordType: RecordType | null; recordTypeKey: string; recordId: string; recordTypes?: RecordType[] }>()
const emit = defineEmits<{ back: []; 'update:recordId': [id: string] }>()

const records = ref<RecordMessage[]>([])
const nextPageToken = ref('')
const loading = ref(false)
const loadError = ref('')
const editing = ref<RecordMessage | 'new' | null>(null)
const texts = reactive<Record<string, string>>({})
const checks = reactive<Record<string, boolean>>({})
const errors = reactive<Record<string, string>>({})
const touched = new Set<string>()
const busy = ref(false)
// The owning record of a new child record.
const parentId = ref('')
// A manual document name, for Record Types that allow one.
const manualName = ref('')
const history = ref<RecordEvent[]>([])
const versions = ref<RecordVersion[]>([])
const activity = ref<ActivityEvent[]>([])
// What the server's expression rules say about each field for the values
// entered so far (PreviewRecord). The server calculates and checks again on save.
const states = reactive<Record<string, FieldState>>({})
const filterText = ref('')
const appliedFilter = ref('')
const filterOpen = ref(false)
let previewTimer: ReturnType<typeof setTimeout> | undefined
let previewRequest = 0
let lastPreview = ''
const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
const formRef = ref<HTMLFormElement | null>(null)
// Each list request gets a number so a slow response for a previous Record Type is ignored.
let listRequest = 0

function message(err: unknown) {
  return err instanceof ConnectError ? err.rawMessage : 'Something went wrong.'
}

async function load(more = false) {
  const recordType = props.recordType
  if (!recordType) { records.value = []; nextPageToken.value = ''; return }
  const request = ++listRequest
  loading.value = true
  loadError.value = ''
  try {
    const res = await recordClient.listRecords({ recordTypeKey: recordType.name, pageSize: 50, pageToken: more ? nextPageToken.value : '', filter: appliedFilter.value })
    if (request !== listRequest) return
    records.value = more ? [...records.value, ...res.records] : res.records
    nextPageToken.value = res.nextPageToken
  } catch (err) {
    if (request !== listRequest) return
    loadError.value = message(err)
    toast.error(`Could not load records: ${loadError.value}`, { action: { label: 'Retry', onClick: () => { void load() } } })
  } finally {
    if (request === listRequest) loading.value = false
  }
}

watch(() => props.recordType?.name, () => { filterText.value = ''; appliedFilter.value = ''; void load() }, { immediate: true })
function applyFilter(text: string) {
  filterText.value = text
  appliedFilter.value = text.trim()
  void load()
}

// Opening an record changes the URL; the watcher below fills the form, so a
// page refresh lands on the same record.
function openRecord(record: RecordMessage | 'new') {
  emit('update:recordId', record === 'new' ? 'new' : record.id)
}

function close() {
  emit('update:recordId', '')
}

watch(() => [props.recordId, props.recordType?.name] as const, async ([id]) => {
  const recordType = props.recordType
  if (!id || !recordType) { editing.value = null; return }
  if (id === 'new') { open('new'); return }
  if (editing.value !== null && editing.value !== 'new' && editing.value.id === id) return
  const known = records.value.find((item) => item.id === id)
  if (known) { open(known); return }
  try {
    const res = await recordClient.getRecord({ recordTypeKey: recordType.name, id })
    if (res.record && props.recordId === id) open(res.record)
  } catch (err) {
    toast.error(message(err))
    close()
  }
}, { immediate: true })

async function loadHistory(record: RecordMessage) {
  const recordType = props.recordType
  // Reading the history needs the audit action, so skip it rather than show an error.
  if (!recordType || !can('audit')) return
  try {
    const res = await recordClient.listRecordHistory({ recordTypeKey: recordType.name, id: record.id })
    if (editing.value !== 'new' && editing.value?.id === record.id) history.value = res.events
    if (!recordType.submittable) return
    const [versionRes, activityRes] = await Promise.all([
      recordClient.listRecordVersions({ recordTypeKey: recordType.name, id: record.id }),
      recordClient.listRecordActivity({ recordTypeKey: recordType.name, id: record.id }),
    ])
    if (editing.value !== 'new' && editing.value?.id === record.id) { versions.value = versionRes.versions; activity.value = activityRes.events }
  } catch (err) {
    toast.error(`Could not load history: ${message(err)}`)
  }
}

const labels = computed(() => new Map(props.recordType?.fields.map((field) => [field.name, field.label]) ?? []))

const historyActions: Record<string, string> = { created: 'Created', updated: 'Updated', deleted: 'Deleted', submitted: gloss('submitted').label, cancelled: gloss('cancelled').label, name_override: 'Name set by hand' }
function describe(event: RecordEvent) {
  const action = historyActions[event.action] ?? event.action
  const fields = event.changedFields.map((key) => labels.value.get(key) ?? key)
  return fields.length > 0 && event.action === 'updated' ? `${action} ${fields.join(', ')}` : action
}
// One line per field of an Audit Log row: the old and new value. Rows imported
// from before the Audit Log name the field only.
function describeChange(change: FieldChange): string {
  const name = labels.value.get(change.field) ?? (change.field === 'docstate' ? 'Status' : change.field === 'name' ? 'Name' : change.field)
  if (change.masked) return `${name}: hidden at your permission level`
  if (change.oldValue === undefined && change.newValue === undefined) return name
  const show = (value: string | undefined) => (value === undefined || value === '' ? 'empty' : change.field === 'docstate' ? stateLabels[Number(value)] ?? value : value)
  return `${name}: ${show(change.oldValue)} → ${show(change.newValue)}`
}

function clear(target: Record<string, unknown>) {
  for (const key of Object.keys(target)) delete target[key]
}

function open(record: RecordMessage | 'new') {
  if (!props.recordType) return
  editing.value = record
  history.value = []
  versions.value = []
  activity.value = []
  clear(texts); clear(checks); clear(errors); clear(states); touched.clear()
  lastPreview = ''
  parentId.value = record === 'new' ? '' : record.parentId
  manualName.value = ''
  for (const field of props.recordType.fields) {
    const kind = record === 'new' ? undefined : record.values[field.name]?.kind
    // A new record shows the default the server will fill in.
    if (field.type === 'Checkbox') checks[field.name] = kind?.case === 'boolValue' ? kind.value : record === 'new' && field.default === 'true'
    else texts[field.name] = kind?.case === 'stringValue' ? kind.value : record === 'new' ? field.default ?? '' : ''
  }
  if (record !== 'new') void loadHistory(record)
  schedulePreview(0)
}

// Read-only fields take only their default, and a set-once field is fixed
// after it has a value. The server enforces both.
function locked(field: Field): boolean {
  if (field.read_only || isCalculated(field) || states[field.name]?.readOnly) return true
  if (!fieldEditable(field, submittable.value, docstate.value)) return true
  if (!field.set_once || editing.value === null || editing.value === 'new') return false
  const kind = editing.value.values[field.name]?.kind
  return kind?.case === 'stringValue' || kind?.case === 'boolValue'
}

// Document lifecycle: only the kernel moves a document between these states.
const submittable = computed(() => props.recordType?.submittable === true)
const docstate = computed(() => editing.value === null || editing.value === 'new' ? docstates.draft : editing.value.docstate)
const stateLabels = [gloss('draft').label, gloss('submitted').label, gloss('cancelled').label]
const stateLabel = computed(() => stateLabels[docstate.value] ?? '')
const editable = computed(() => !submittable.value || docstate.value !== docstates.cancelled)
const isExisting = computed(() => editing.value !== null && editing.value !== 'new')
const activityLabels: Record<string, string> = { submitted: gloss('submitted').label, cancelled: gloss('cancelled').label, amended: gloss('amended').label, amended_from: 'Created by amending', edited_after_submit: 'Edited after submit' }
function describeActivity(event: ActivityEvent) {
  const fields = event.changedFields.map((key) => labels.value.get(key) ?? key)
  const label = activityLabels[event.event] ?? event.event
  return fields.length > 0 ? `${label}: ${fields.join(', ')}` : label
}
const isChild = computed(() => props.recordType?.kind === 'child')
const isSetting = computed(() => props.recordType?.kind === 'setting')
const parentKey = computed(() => props.recordType?.parent_record_type ?? '')
const parentLabel = computed(() => props.recordTypes?.find((item) => item.name === parentKey.value)?.label ?? 'owning record')
// A Link field names its target by key; the picker shows the Record Type's label.
function targetLabel(field: Field): string {
  return props.recordTypes?.find((item) => item.name === field.target_record_type)?.label ?? field.target_record_type ?? 'linked record'
}
const allowsManualName = computed(() => props.recordType?.allow_manual_name === true && props.recordType.submittable === true)
// A Setting holds one record, so creating is offered only while it is empty.
// can reports whether the signed-in user's Grants allow an action here. The
// server checks again on every call; this only hides what would be refused.
function can(action: string): boolean {
  return props.recordType?.access?.actions.includes(action) ?? true
}
const canCreate = computed(() => can('create') && (!isSetting.value || (records.value.length === 0 && !loading.value && !loadError.value)))

function fieldLabel(field: Field): string {
  if (field.type === 'Money' && field.currency) return `${field.label} (${field.currency})`
  if (field.type === 'Quantity' && field.uom) return `${field.label} (${field.uom})`
  return field.label
}

// A field may be required by its required_if rule as well as by its own flag.
function effective(field: Field): Field {
  return states[field.name]?.required && !field.required ? { ...field, required: true } : field
}
const shownFields = computed(() => (props.recordType?.fields ?? []).filter((field) => !states[field.name]?.hidden))
const hasExpressions = computed(() => props.recordType?.fields.some((field) => field.formula || field.required_if || field.read_only_if || field.validate || field.depends_on) ?? false)
function schedulePreview(delay = 250) {
  clearTimeout(previewTimer)
  if (!hasExpressions.value || !editing.value) return
  previewTimer = setTimeout(() => { void preview() }, delay)
}
async function preview() {
  const recordType = props.recordType
  const current = editing.value
  if (!recordType || !current) return
  const values = Object.fromEntries(recordType.fields.filter((field) => !isCalculated(field)).map((field) => [field.name, toValue(field)]))
  const id = current === 'new' ? '' : current.id
  const payload = JSON.stringify([id, Object.entries(values).map(([key, value]) => [key, value.kind.value])])
  if (payload === lastPreview) return
  lastPreview = payload
  const request = ++previewRequest
  try {
    const res = await recordClient.previewRecord({ recordTypeKey: recordType.name, id, values })
    if (request !== previewRequest || editing.value !== current) return
    for (const [key, value] of Object.entries(res.computed)) texts[key] = value.kind.case === 'stringValue' ? value.kind.value : ''
    clear(states)
    for (const state of res.states) states[state.key] = state
  } catch {
    // The preview is a hint; the server still checks every save.
  }
}
watch([texts, checks], () => schedulePreview(), { deep: true })
function check(field: Field) {
  const problem = validateField(effective(field), texts[field.name] ?? '')
  if (problem) errors[field.name] = problem
  else delete errors[field.name]
}

function onBlur(field: Field) {
  touched.add(field.name)
  check(field)
}

function onInput(field: Field) {
  if (touched.has(field.name) || errors[field.name]) check(field)
}

async function focusFirstError() {
  await nextTick()
  formRef.value?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
}

// Server validation messages start with the field label; show them under that field.
function placeServerError(err: unknown): boolean {
  if (!(err instanceof ConnectError) || err.code !== Code.InvalidArgument || !props.recordType) return false
  if (isChild.value && /owner|owns it|parent/i.test(err.rawMessage)) {
    errors.parent_id = `${err.rawMessage.charAt(0).toUpperCase()}${err.rawMessage.slice(1)}.`
    void focusFirstError()
    return true
  }
  if (/name/i.test(err.rawMessage) && allowsManualName.value && editing.value === 'new' && !props.recordType.fields.some((item) => err.rawMessage.startsWith(item.label))) {
    errors.manual_name = `${err.rawMessage.charAt(0).toUpperCase()}${err.rawMessage.slice(1)}.`
    void focusFirstError()
    return true
  }
  const field = props.recordType.fields.find((item) => err.rawMessage.startsWith(item.label))
  if (!field) return false
  errors[field.name] = `${err.rawMessage.charAt(0).toUpperCase()}${err.rawMessage.slice(1)}.`
  void focusFirstError()
  return true
}

function reportWriteError(err: unknown) {
  if (placeServerError(err)) return
  if (err instanceof ConnectError && err.code === Code.Aborted) toast.error(message(err), { action: { label: 'Reload', onClick: () => { void reloadRecord() } } })
  else toast.error(message(err))
}

function toValue(field: Field): FieldValue {
  if (field.type === 'Checkbox') return create(FieldValueSchema, { kind: { case: 'boolValue', value: checks[field.name] ?? false } })
  const text = (texts[field.name] ?? '').trim()
  return create(FieldValueSchema, text ? { kind: { case: 'stringValue', value: text } } : { kind: { case: 'nullValue', value: true } })
}

async function submit() {
  const recordType = props.recordType
  if (!editing.value || !recordType || !keyMatchesRoute(recordType.name, props.recordTypeKey)) return
  for (const field of recordType.fields) { touched.add(field.name); if (!locked(field) && !states[field.name]?.hidden) check(field) }
  if (editing.value === 'new' && isChild.value && !parentId.value.trim()) errors.parent_id = `Choose the ${parentLabel.value} this belongs to.`
  if (editing.value === 'new' && allowsManualName.value) {
    const problem = validateManualName(manualName.value)
    if (problem) errors.manual_name = problem
  }
  if (Object.keys(errors).length > 0) { await focusFirstError(); return }
  busy.value = true
  const values = Object.fromEntries(recordType.fields.filter((field) => !locked(field) && !states[field.name]?.hidden).map((field) => [field.name, toValue(field)]))
  try {
    if (editing.value === 'new') {
      const res = await recordClient.createRecord({ recordTypeKey: props.recordTypeKey, values, parentId: isChild.value ? parentId.value.trim() : '', name: allowsManualName.value ? manualName.value.trim() : '' })
      if (res.record) records.value = [...records.value, res.record]
      forgetLabels(recordType.name)
      toast.success('Record created.')
      close()
    } else {
      const current = editing.value
      const res = await recordClient.updateRecord({ recordTypeKey: props.recordTypeKey, id: current.id, expectedRevision: current.revision, values })
      const updated = res.record
      if (updated) {
        records.value = records.value.map((item) => item.id === current.id ? updated : item)
        open(updated)
      }
      forgetLabels(recordType.name)
      toast.success('Record saved.')
    }
  } catch (err) {
    reportWriteError(err)
  } finally {
    busy.value = false
  }
}

type Transition = 'submit' | 'cancel' | 'amend'
const transitionText: Record<Transition, string> = { submit: 'Document submitted.', cancel: 'Document cancelled.', amend: 'Amended draft created.' }
const confirmingSubmit = ref(false)
const confirmingCancel = ref(false)
const confirmingDelete = ref(false)
async function runTransition(action: Transition) {
  const recordType = props.recordType
  if (!editing.value || editing.value === 'new' || !recordType) return
  const current = editing.value
  const request = { recordTypeKey: recordType.name, id: current.id, expectedRevision: current.revision }
  busy.value = true
  try {
    const record = action === 'submit' ? (await recordClient.submitRecord(request)).record
      : action === 'cancel' ? (await recordClient.cancelRecord(request)).record
      : (await recordClient.amendRecord(request)).record
    if (record) {
      const fresh = record
      // An amend returns the new draft; the other two return the changed document.
      if (action === 'amend') {
        const original = await recordClient.getRecord({ recordTypeKey: recordType.name, id: current.id })
        records.value = [...records.value.map((item) => item.id === current.id && original.record ? original.record : item), fresh]
        emit('update:recordId', fresh.id)
      } else {
        records.value = records.value.map((item) => item.id === fresh.id ? fresh : item)
        open(fresh)
      }
    }
    forgetLabels(recordType.name)
    toast.success(transitionText[action])
  } catch (err) {
    reportWriteError(err)
  } finally {
    confirmingSubmit.value = false
    confirmingCancel.value = false
    busy.value = false
  }
}
async function reloadRecord() {
  const recordType = props.recordType
  if (!editing.value || editing.value === 'new' || !recordType) return
  try {
    const res = await recordClient.getRecord({ recordTypeKey: recordType.name, id: editing.value.id })
    if (res.record) {
      const fresh = res.record
      records.value = records.value.map((item) => item.id === fresh.id ? fresh : item)
      open(fresh)
      toast.info('Record reloaded with the latest changes.')
    }
  } catch (err) {
    toast.error(message(err))
  }
}

async function remove() {
  const recordType = props.recordType
  if (!editing.value || editing.value === 'new' || !recordType) return
  const current = editing.value
  busy.value = true
  try {
    await recordClient.deleteRecord({ recordTypeKey: recordType.name, id: current.id, expectedRevision: current.revision })
    records.value = records.value.filter((item) => item.id !== current.id)
    forgetLabels(recordType.name)
    toast.success('Record deleted.')
    close()
  } catch (err) {
    reportWriteError(err)
  } finally {
    confirmingDelete.value = false
    busy.value = false
  }
}

function inputType(field: Field) {
  return field.type === 'Email' ? 'email' : field.type === 'Phone' ? 'tel' : 'text'
}

// A calculated field shows the server's formula error until it validates.
function calculatedError(field: Field): string | undefined {
  return states[field.name]?.error && isCalculated(field) ? states[field.name]?.error : undefined
}
</script>

<template>
  <section class="page-content view-page">
    <div v-if="!recordType" class="empty-builder">
      <h2>Nothing to view yet</h2>
      <p>Define the first form in Build, then add its records here.</p>
      <ActionButton kind="secondary" label="Go to Build" icon="hammer" @press="emit('back')" />
    </div>

    <template v-else-if="editing">
      <button type="button" class="back-link" @click="close">← {{ recordType.label }}</button>
      <div class="page-title-row">
        <div class="entry-status">
          <h1>{{ editing === 'new' ? 'New record' : 'Edit record' }}</h1>
          <Badge v-if="submittable && isExisting" :variant="docstate === 2 ? 'destructive' : docstate === 1 ? 'default' : 'secondary'">{{ stateLabel }}</Badge>
          <small v-if="submittable && isExisting && editing !== 'new' && editing.name !== editing.id" class="muted-label">{{ editing.name }}</small>
        </div>
        <div v-if="submittable && editing !== 'new'" class="lifecycle-actions">
          <ActionButton v-if="docstate === 0" kind="primary" label="Submit" icon="send" :busy="busy" @press="confirmingSubmit = true" />
          <ActionButton v-if="docstate === 1" kind="destructive" label="Cancel document" icon="x" :busy="busy" @press="confirmingCancel = true" />
          <ActionButton v-if="docstate === 2 && recordType.amendable" kind="restore" label="Amend" icon="rotate-ccw" :busy="busy" @press="runTransition('amend')" />
          <ConfirmDialog
            v-model:open="confirmingSubmit"
            title="Submit this document?"
            body="After it is submitted only fields set to allow editing can change. Save any edits first."
            confirm-label="Submit document"
            cancel-label="Not yet"
            confirm-icon="send"
            :danger="false"
            :busy="busy"
            @confirm="runTransition('submit')"
          />
          <ConfirmDialog
            v-model:open="confirmingCancel"
            title="Cancel this document?"
            body="It stays visible as cancelled and cannot be edited. A document that a submitted document links to cannot be cancelled."
            confirm-label="Cancel document"
            cancel-label="Keep it"
            confirm-icon="x"
            :busy="busy"
            @confirm="runTransition('cancel')"
          />
        </div>
      </div>
      <form ref="formRef" class="entry-form" novalidate @submit.prevent="submit">
        <div class="entry-form-grid">
          <LinkField
            v-if="isChild"
            id="entry-parent-id"
            :label="`Belongs to (${parentLabel})`"
            :error="errors.parent_id"
            v-model="parentId"
            :record-type-key="parentKey"
            :target-label="parentLabel"
            :disabled="editing !== 'new'"
            @update:model-value="delete errors.parent_id"
          />
          <TextField
            v-if="allowsManualName && editing === 'new'"
            id="entry-manual-name"
            label="Name"
            optional
            placeholder="Leave empty to number it when submitted"
            autocomplete="off"
            :error="errors.manual_name"
            v-model="manualName"
            @update:model-value="delete errors.manual_name"
          />
          <template v-for="field in shownFields" :key="field.name">
            <LinkField
              v-if="field.type === 'Link' && field.target_record_type"
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="texts[field.name]"
              :record-type-key="field.target_record_type"
              :target-label="targetLabel(field)"
              :disabled="locked(field)"
              @update:model-value="onInput(field)"
              @blur="onBlur(field)"
            />
            <CheckboxField
              v-else-if="field.type === 'Checkbox'"
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="checks[field.name]"
              :disabled="locked(field)"
            />
            <SelectField
              v-else-if="field.type === 'Enum'"
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="texts[field.name]"
              :options="field.values ?? []"
              :disabled="locked(field)"
              @update:model-value="onInput(field)"
              @blur="onBlur(field)"
            />
            <LongTextField
              v-else-if="field.type === 'Long text'"
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="texts[field.name]"
              class="is-wide"
              :disabled="locked(field)"
              @update:model-value="onInput(field)"
              @blur="onBlur(field)"
            />
            <DateField
              v-else-if="field.type === 'Date'"
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="texts[field.name]"
              :disabled="locked(field)"
              @update:model-value="onInput(field)"
              @blur="onBlur(field)"
            />
            <NumberField
              v-else-if="['Number', 'Currency', 'Money', 'Quantity'].includes(field.type)"
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="texts[field.name]"
              :disabled="locked(field)"
              @update:model-value="onInput(field)"
              @blur="onBlur(field)"
            />
            <TextField
              v-else
              :id="`entry-${field.name}`"
              :label="fieldLabel(field)"
              :optional="!effective(field).required"
              :error="errors[field.name] ?? calculatedError(field)"
              v-model="texts[field.name]"
              :type="inputType(field)"
              :disabled="locked(field)"
              @update:model-value="onInput(field)"
              @blur="onBlur(field)"
            />
          </template>
        </div>
        <div class="entry-form-footer">
          <ActionButton v-if="editing !== 'new' && !isSetting && (!submittable || docstate === 0)" kind="destructive" label="Delete" icon="trash-2" :busy="busy" @press="confirmingDelete = true" />
          <ConfirmDialog
            v-model:open="confirmingDelete"
            title="Delete this record?"
            :body="recordType.kind === 'master' ? 'It disappears from lists. The row is kept, so its history stays.' : 'This cannot be undone.'"
            confirm-label="Delete record"
            :busy="busy"
            @confirm="remove"
          />
          <div class="toolbar-actions"><ActionButton kind="secondary" label="Cancel" icon="x" :busy="busy" @press="close" /><ActionButton v-if="editable" type="submit" kind="primary" :label="editing === 'new' ? 'Create record' : 'Save'" :icon="editing === 'new' ? 'plus' : 'save'" :busy="busy" /></div>
        </div>
      </form>
      <section v-if="editing !== 'new'" class="entry-meta" aria-labelledby="entry-details-heading">
        <h2 id="entry-details-heading">Details</h2>
        <div class="entry-markers">
          <Marker variant="border"><MarkerIcon><CalendarPlus /></MarkerIcon><MarkerContent><span class="marker-label">Created</span><span :title="formatWhenFull(editing.createdAt)">{{ formatWhen(editing.createdAt) }}</span></MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><Clock /></MarkerIcon><MarkerContent><span class="marker-label">Last updated</span><span :title="formatWhenFull(editing.updatedAt)">{{ formatWhen(editing.updatedAt) }}</span></MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><Hash /></MarkerIcon><MarkerContent><span class="marker-label">Version</span>{{ editing.revision }}</MarkerContent></Marker>
          <Marker v-if="editing.createdBy" variant="border"><MarkerIcon><User /></MarkerIcon><MarkerContent><span class="marker-label">Created by</span>{{ editing.createdBy }}</MarkerContent></Marker>
          <Marker v-if="editing.owner" variant="border"><MarkerIcon><User /></MarkerIcon><MarkerContent><span class="marker-label">Owner</span>{{ editing.owner }}</MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><Layers /></MarkerIcon><MarkerContent><span class="marker-label">{{ gloss('formVersion').label }}</span>{{ editing.defVersion }}</MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><Fingerprint /></MarkerIcon><MarkerContent><span class="marker-label">Record ID</span><span class="is-mono">{{ editing.id }}</span></MarkerContent></Marker>
        </div>
        <template v-if="submittable">
          <h2>Activity</h2>
          <ol class="entry-markers">
            <li v-for="event in activity" :key="`${event.event}:${event.at}:${event.relatedId}`">
              <Marker variant="border"><MarkerIcon><Clock /></MarkerIcon><MarkerContent><span class="marker-label">{{ describeActivity(event) }}</span>{{ event.actor }} · {{ when.format(new Date(event.at)) }}</MarkerContent></Marker>
            </li>
          </ol>
          <h2>Versions</h2>
          <ol class="entry-markers">
            <li v-for="version in versions" :key="String(version.version)">
              <Marker variant="border"><MarkerIcon><Layers /></MarkerIcon><MarkerContent><span class="marker-label">Version {{ version.version }}: {{ version.event }}</span>{{ version.actor }} · {{ when.format(new Date(version.at)) }} · revision {{ version.revision }}</MarkerContent></Marker>
            </li>
          </ol>
        </template>
        <h2>{{ gloss('history').label }}</h2>
        <ol class="entry-markers">
          <li v-for="event in history" :key="`${event.revision}:${event.action}:${event.at}`">
            <Marker variant="border">
              <MarkerIcon><Plus v-if="event.action === 'created'" /><Trash2 v-else-if="event.action === 'deleted'" /><Pencil v-else /></MarkerIcon>
              <MarkerContent><span class="marker-label">{{ describe(event) }}</span>{{ event.actor }} · {{ when.format(new Date(event.at)) }} · revision {{ event.revision }}
                <ul v-if="event.changes.length > 0" class="audit-changes"><li v-for="change in event.changes" :key="change.field">{{ describeChange(change) }}</li></ul>
              </MarkerContent>
            </Marker>
          </li>
        </ol>
      </section>
    </template>

    <template v-else>
      <div class="view-head"><div><h1>{{ recordType.label }}</h1><p v-if="recordType.description" class="lead">{{ recordType.description }}</p></div><ActionButton v-if="canCreate" kind="primary" label="New record" icon="plus" @press="openRecord('new')" /></div>
      <form class="entries-filter" role="search" @submit.prevent="applyFilter(filterText)">
        <Input :model-value="filterText" aria-label="Filter records with an expression" placeholder='Filter, for example doc.total > dec("100")' @update:model-value="filterText = String($event)" />
        <ActionButton type="submit" kind="secondary" density="row" label="Filter" icon="list-filter" />
        <ActionButton v-if="appliedFilter" kind="quiet" density="row" label="Clear" icon="x" @press="applyFilter('')" />
      </form>
      <div class="mb-3 min-[640px]:hidden">
        <ActionButton kind="secondary" :label="appliedFilter ? 'Filtered' : 'Filter'" icon="list-filter" full-bleed @press="filterOpen = true" />
        <BottomDrawer v-model:open="filterOpen" title="Filter records">
          <TextField id="entries-filter-text" label="Filter expression" optional v-model="filterText" placeholder='For example doc.total > dec("100")' help="Only matching records list." />
          <template #footer>
            <ActionButton v-if="appliedFilter" kind="quiet" label="Clear" icon="x" @press="applyFilter(''); filterOpen = false" />
            <ActionButton kind="primary" label="Apply filter" icon="list-filter" full-bleed @press="applyFilter(filterText); filterOpen = false" />
          </template>
        </BottomDrawer>
      </div>
      <PullToRefresh :refreshing="loading" @refresh="load()">
        <div class="entries-panel">
          <RecordsTable v-if="records.length > 0" :key="recordType.name" :record-type-key="recordType.name" :fields="recordType.fields" :records="records" :submittable="submittable" @open="openRecord" />
          <p v-else-if="loading && records.length === 0" class="empty-state">Loading…</p>
          <p v-else-if="loadError" class="empty-state">Records could not be loaded. <ActionButton kind="secondary" density="row" label="Retry" icon="refresh-ccw" @press="load()" /></p>
          <div v-else class="entries-empty"><p>{{ appliedFilter ? 'No records match the filter.' : 'No records yet.' }}</p><ActionButton v-if="canCreate && !appliedFilter" kind="primary" label="Create the first record" icon="plus" @press="openRecord('new')" /></div>
          <div v-if="nextPageToken && !loading" class="entries-more"><ActionButton kind="secondary" density="row" label="Load more" icon="chevron-down" @press="load(true)" /></div>
        </div>
      </PullToRefresh>
    </template>
  </section>
</template>
