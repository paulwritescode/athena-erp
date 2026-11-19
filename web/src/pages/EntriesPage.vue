<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { entryClient } from '../api'
import { validateField } from '../fieldRules'
import { FieldValueSchema, type Entry, type EntryEvent, type FieldValue } from '../gen/athena/v1/entries_pb'
import type { Field, RecordDefinition } from '../types'
import { Button } from '@/components/ui/button'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog'
import EntriesTable from '../components/EntriesTable.vue'
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker'
import { CalendarPlusIcon, ClockIcon, FingerprintIcon, HashIcon, PencilIcon, PlusIcon, Trash2Icon } from '@lucide/vue'
import { toast } from 'vue-sonner'

// record is null when no saved Record exists yet. entryId comes from the page
// URL: '' for the table, 'new' for the create form, or an entry ID.
const props = defineProps<{ record: RecordDefinition | null; entryId: string }>()
const emit = defineEmits<{ back: []; 'update:entryId': [id: string] }>()

const entries = ref<Entry[]>([])
const nextPageToken = ref('')
const loading = ref(false)
const loadError = ref('')
const editing = ref<Entry | 'new' | null>(null)
const texts = reactive<Record<string, string>>({})
const checks = reactive<Record<string, boolean>>({})
const errors = reactive<Record<string, string>>({})
const touched = new Set<string>()
const busy = ref(false)
const history = ref<EntryEvent[]>([])
const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
const formRef = ref<HTMLFormElement | null>(null)
// Each list request gets a number so a slow response for a previous Record is ignored.
let listRequest = 0

function message(err: unknown) {
  return err instanceof ConnectError ? err.rawMessage : 'Something went wrong.'
}

async function load(more = false) {
  const record = props.record
  if (!record) { entries.value = []; nextPageToken.value = ''; return }
  const request = ++listRequest
  loading.value = true
  loadError.value = ''
  try {
    const res = await entryClient.listEntries({ recordKey: record.name, pageSize: 50, pageToken: more ? nextPageToken.value : '' })
    if (request !== listRequest) return
    entries.value = more ? [...entries.value, ...res.entries] : res.entries
    nextPageToken.value = res.nextPageToken
  } catch (err) {
    if (request !== listRequest) return
    loadError.value = message(err)
    toast.error(`Could not load entries: ${loadError.value}`, { action: { label: 'Retry', onClick: () => { void load() } } })
  } finally {
    if (request === listRequest) loading.value = false
  }
}

watch(() => props.record?.name, () => { void load() }, { immediate: true })

// Opening an entry changes the URL; the watcher below fills the form, so a
// page refresh lands on the same entry.
function openEntry(entry: Entry | 'new') {
  emit('update:entryId', entry === 'new' ? 'new' : entry.id)
}

function close() {
  emit('update:entryId', '')
}

watch(() => [props.entryId, props.record?.name] as const, async ([id]) => {
  const record = props.record
  if (!id || !record) { editing.value = null; return }
  if (id === 'new') { open('new'); return }
  if (editing.value !== null && editing.value !== 'new' && editing.value.id === id) return
  const known = entries.value.find((item) => item.id === id)
  if (known) { open(known); return }
  try {
    const res = await entryClient.getEntry({ recordKey: record.name, id })
    if (res.entry && props.entryId === id) open(res.entry)
  } catch (err) {
    toast.error(message(err))
    close()
  }
}, { immediate: true })

async function loadHistory(entry: Entry) {
  const record = props.record
  if (!record) return
  try {
    const res = await entryClient.listEntryHistory({ recordKey: record.name, id: entry.id })
    if (editing.value !== 'new' && editing.value?.id === entry.id) history.value = res.events
  } catch (err) {
    toast.error(`Could not load history: ${message(err)}`)
  }
}

const labels = computed(() => new Map(props.record?.fields.map((field) => [field.name, field.label]) ?? []))

function describe(event: EntryEvent) {
  const action = event.action === 'created' ? 'Created' : event.action === 'deleted' ? 'Deleted' : 'Updated'
  const fields = event.changedFields.map((key) => labels.value.get(key) ?? key)
  return fields.length > 0 && event.action === 'updated' ? `${action} ${fields.join(', ')}` : action
}

function clear(target: Record<string, unknown>) {
  for (const key of Object.keys(target)) delete target[key]
}

function open(entry: Entry | 'new') {
  if (!props.record) return
  editing.value = entry
  history.value = []
  clear(texts); clear(checks); clear(errors); touched.clear()
  for (const field of props.record.fields) {
    const kind = entry === 'new' ? undefined : entry.values[field.name]?.kind
    if (field.type === 'Checkbox') checks[field.name] = kind?.case === 'boolValue' ? kind.value : false
    else texts[field.name] = kind?.case === 'stringValue' ? kind.value : ''
  }
  if (entry !== 'new') void loadHistory(entry)
}

function check(field: Field) {
  const problem = validateField(field, texts[field.name] ?? '')
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
  if (!(err instanceof ConnectError) || err.code !== Code.InvalidArgument || !props.record) return false
  const field = props.record.fields.find((item) => err.rawMessage.startsWith(item.label))
  if (!field) return false
  errors[field.name] = `${err.rawMessage.charAt(0).toUpperCase()}${err.rawMessage.slice(1)}.`
  void focusFirstError()
  return true
}

function reportWriteError(err: unknown) {
  if (placeServerError(err)) return
  if (err instanceof ConnectError && err.code === Code.Aborted) toast.error(message(err), { action: { label: 'Reload', onClick: () => { void reloadEntry() } } })
  else toast.error(message(err))
}

function toValue(field: Field): FieldValue {
  if (field.type === 'Checkbox') return create(FieldValueSchema, { kind: { case: 'boolValue', value: checks[field.name] ?? false } })
  const text = (texts[field.name] ?? '').trim()
  return create(FieldValueSchema, text ? { kind: { case: 'stringValue', value: text } } : { kind: { case: 'nullValue', value: true } })
}

async function submit() {
  const record = props.record
  if (!editing.value || !record) return
  for (const field of record.fields) { touched.add(field.name); check(field) }
  if (Object.keys(errors).length > 0) { await focusFirstError(); return }
  busy.value = true
  const values = Object.fromEntries(record.fields.map((field) => [field.name, toValue(field)]))
  try {
    if (editing.value === 'new') {
      const res = await entryClient.createEntry({ recordKey: record.name, values })
      if (res.entry) entries.value = [...entries.value, res.entry]
      toast.success('Entry created.')
      close()
    } else {
      const current = editing.value
      const res = await entryClient.updateEntry({ recordKey: record.name, id: current.id, expectedRevision: current.revision, values })
      const updated = res.entry
      if (updated) {
        entries.value = entries.value.map((item) => item.id === current.id ? updated : item)
        open(updated)
      }
      toast.success('Entry saved.')
    }
  } catch (err) {
    reportWriteError(err)
  } finally {
    busy.value = false
  }
}

async function reloadEntry() {
  const record = props.record
  if (!editing.value || editing.value === 'new' || !record) return
  try {
    const res = await entryClient.getEntry({ recordKey: record.name, id: editing.value.id })
    if (res.entry) {
      const fresh = res.entry
      entries.value = entries.value.map((item) => item.id === fresh.id ? fresh : item)
      open(fresh)
      toast.info('Entry reloaded with the latest changes.')
    }
  } catch (err) {
    toast.error(message(err))
  }
}

async function remove() {
  const record = props.record
  if (!editing.value || editing.value === 'new' || !record) return
  const current = editing.value
  busy.value = true
  try {
    await entryClient.deleteEntry({ recordKey: record.name, id: current.id, expectedRevision: current.revision })
    entries.value = entries.value.filter((item) => item.id !== current.id)
    toast.success('Entry deleted.')
    close()
  } catch (err) {
    reportWriteError(err)
  } finally {
    busy.value = false
  }
}

function inputType(field: Field) {
  return field.type === 'Email' ? 'email' : field.type === 'Date' ? 'date' : field.type === 'Phone' ? 'tel' : 'text'
}
</script>

<template>
  <section class="page-content view-page">
    <div v-if="!record" class="empty-builder">
      <h2>Nothing to view yet</h2>
      <p>Create and save a Record in Build, then add its entries here.</p>
      <Button variant="outline" @click="emit('back')">Go to Build</Button>
    </div>

    <template v-else-if="editing">
      <button type="button" class="back-link" @click="close">← {{ record.label }}</button>
      <div class="page-title-row"><h1>{{ editing === 'new' ? 'New entry' : 'Edit entry' }}</h1></div>
      <form ref="formRef" class="entry-form" novalidate @submit.prevent="submit">
        <div class="entry-form-grid">
          <div v-for="field in record.fields" :key="field.name" class="form-field" :class="{ 'is-wide': field.type === 'Long text', 'is-invalid': errors[field.name] }">
            <label :for="`entry-${field.name}`" class="form-label">{{ field.label }}<span v-if="field.required" class="required-mark" aria-hidden="true">*</span></label>
            <label v-if="field.type === 'Checkbox'" class="checkbox-row"><input :id="`entry-${field.name}`" v-model="checks[field.name]" type="checkbox" /><span>{{ checks[field.name] ? 'Yes' : 'No' }}</span></label>
            <textarea v-else-if="field.type === 'Long text'" :id="`entry-${field.name}`" v-model="texts[field.name]" rows="4" :aria-invalid="errors[field.name] ? 'true' : undefined" :aria-required="field.required || undefined" :aria-describedby="errors[field.name] ? `entry-${field.name}-error` : undefined" @blur="onBlur(field)" @input="onInput(field)" />
            <input v-else :id="`entry-${field.name}`" v-model="texts[field.name]" :type="inputType(field)" :inputmode="field.type === 'Number' || field.type === 'Currency' ? 'decimal' : undefined" :placeholder="field.type === 'Link' ? `${field.target_record} entry ID` : undefined" :aria-invalid="errors[field.name] ? 'true' : undefined" :aria-required="field.required || undefined" :aria-describedby="errors[field.name] ? `entry-${field.name}-error` : undefined" @blur="onBlur(field)" @input="onInput(field)" />
            <p v-if="errors[field.name]" :id="`entry-${field.name}-error`" class="field-error">{{ errors[field.name] }}</p>
          </div>
        </div>
        <div class="entry-form-footer">
          <AlertDialog v-if="editing !== 'new'">
            <AlertDialogTrigger as-child><Button type="button" variant="ghost" :disabled="busy">Delete</Button></AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete this entry?</AlertDialogTitle>
                <AlertDialogDescription>This cannot be undone.</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction variant="destructive" @click="remove">Delete entry</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
          <div class="toolbar-actions"><Button type="button" variant="outline" @click="close">Cancel</Button><Button type="submit" :disabled="busy">{{ editing === 'new' ? 'Create entry' : 'Save' }}</Button></div>
        </div>
      </form>
      <section v-if="editing !== 'new'" class="entry-meta" aria-labelledby="entry-details-heading">
        <h2 id="entry-details-heading">Details</h2>
        <div class="entry-markers">
          <Marker variant="border"><MarkerIcon><CalendarPlusIcon /></MarkerIcon><MarkerContent><span class="marker-label">Created</span>{{ when.format(new Date(editing.createdAt)) }}</MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><ClockIcon /></MarkerIcon><MarkerContent><span class="marker-label">Last updated</span>{{ when.format(new Date(editing.updatedAt)) }}</MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><HashIcon /></MarkerIcon><MarkerContent><span class="marker-label">Revision</span>{{ editing.revision }}</MarkerContent></Marker>
          <Marker variant="border"><MarkerIcon><FingerprintIcon /></MarkerIcon><MarkerContent><span class="marker-label">Entry ID</span><span class="is-mono">{{ editing.id }}</span></MarkerContent></Marker>
        </div>
        <h2>History</h2>
        <ol class="entry-markers">
          <li v-for="event in history" :key="`${event.revision}:${event.action}:${event.at}`">
            <Marker variant="border">
              <MarkerIcon><PlusIcon v-if="event.action === 'created'" /><Trash2Icon v-else-if="event.action === 'deleted'" /><PencilIcon v-else /></MarkerIcon>
              <MarkerContent><span class="marker-label">{{ describe(event) }}</span>{{ event.actor }} · {{ when.format(new Date(event.at)) }} · revision {{ event.revision }}</MarkerContent>
            </Marker>
          </li>
        </ol>
      </section>
    </template>

    <template v-else>
      <div class="view-head"><div><h1>{{ record.label }}</h1><p v-if="record.description" class="lead">{{ record.description }}</p></div><Button @click="openEntry('new')">New entry</Button></div>
      <div class="entries-panel">
        <EntriesTable v-if="entries.length > 0" :key="record.name" :record-key="record.name" :fields="record.fields" :entries="entries" @open="openEntry" />
        <p v-else-if="loading" class="empty-state">Loading…</p>
        <p v-else-if="loadError" class="empty-state">Entries could not be loaded. <Button size="sm" variant="outline" @click="load()">Retry</Button></p>
        <div v-else class="entries-empty"><p>No entries yet.</p><Button variant="outline" size="sm" @click="openEntry('new')">Create the first entry</Button></div>
        <div v-if="nextPageToken && !loading" class="entries-more"><Button variant="outline" size="sm" @click="load(true)">Load more</Button></div>
      </div>
    </template>
  </section>
</template>
