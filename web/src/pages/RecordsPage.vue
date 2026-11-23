<script lang="ts">
// Plain View words for the books-entry UI (G6.10), kept here so the one
// test file covers them without mounting the page. The aliased import is
// the same ConnectError the setup block imports: the two blocks compile
// into one module, so the name cannot repeat.
import { ConnectError as ConnectFailure } from '@connectrpc/connect'

/** Plain words for one posting attempt outcome. Unknown outcomes read as a capitalized word, never raw alone. */
export function postingOutcomeLabel(outcome: string): string {
  switch (outcome) {
    case 'posted': return 'Recorded'
    case 'replayed': return 'Already recorded'
    case 'conflict': return 'Conflict'
    case 'rejected': return 'Blocked'
    case 'error': return 'Failed'
    default: return outcome ? outcome.charAt(0).toUpperCase() + outcome.slice(1) : 'Attempt'
  }
}

// A zero or empty leg amount shows as a dash, so the other column carries
// the row. Amounts are decimal strings from the server, shown exactly.
const zeroAmount = /^-?0+(\.0+)?$/

/** The amount text for one leg, or '' when the cell shows a dash. */
export function ledgerAmount(value: string): string {
  const trimmed = value.trim()
  return trimmed === '' || zeroAmount.test(trimmed) ? '' : trimmed
}

// The reason after the server's "cannot post: " prefix, capitalized, or
// null when the text is not a posting refusal (see postingError in
// internal/server/lifecycle.go).
function postingReason(text: string): string | null {
  const at = text.indexOf('cannot post: ')
  if (at < 0) return null
  const reason = text.slice(at + 'cannot post: '.length).trim()
  if (!reason) return ''
  const sentence = reason.charAt(0).toUpperCase() + reason.slice(1)
  return /[.!?]$/.test(sentence) ? sentence : `${sentence}.`
}

/**
 * The plain sentence a toast shows when the books entry blocks a submit
 * or cancel, or null when the failure is not a posting failure (which
 * keeps its own error path).
 */
export function postingBlockedMessage(err: unknown, action: 'submit' | 'cancel'): string | null {
  if (!(err instanceof ConnectFailure)) return null
  if (err.rawMessage.includes('changed since it was previewed')) {
    return 'The books entry changed since you previewed it. Review the new preview and submit again.'
  }
  const reason = postingReason(err.rawMessage)
  if (reason === null) return null
  const lead = action === 'submit' ? 'Not submitted: the books entry was blocked.' : 'Not cancelled: the reversal was blocked.'
  return reason ? `${lead} ${reason}` : lead
}

/** The plain sentence the preview dialog shows when the books entry is blocked, or null for other failures. */
export function postingPreviewBlocked(err: unknown): string | null {
  if (!(err instanceof ConnectFailure)) return null
  const reason = postingReason(err.rawMessage)
  if (reason === null) return null
  return reason ? `The books entry is blocked. ${reason}` : 'The books entry is blocked.'
}
</script>
<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { ledgerClient, recordClient } from '../api'
import { keyMatchesRoute } from '../composables/useRecordTypes'
import { docstates, fieldEditable, isCalculated, validateField, validateImageValue, validateManualName, type ImageFieldValue } from '../fieldRules'
import { itemSpanClass, planSections, type PlannedSection } from '../forms/layout'
import { gloss } from '../copy/glossary'
import { forgetLabels, labelWithState, resolveRecords } from '../lookup'
import { formatWhen, formatWhenFull } from '../times'
import { FieldValueSchema, type ActivityEvent, type FieldChange, type FieldState, type Record as RecordMessage, type RecordEvent, type RecordVersion, type FieldValue } from '../gen/athena/v1/records_pb'
import type { LedgerRow, PostingGroupStatus, PostingLogEntry, PostingPlan, PostingPlanLine } from '../gen/athena/v1/ledger_pb'
import type { Field, RecordType } from '../types'
import { Badge } from '@/components/ui/badge'
import ActionButton from '@/components/ActionButton.vue'
import type { ActionItem } from '@/components/actions'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type TableFilter } from '@/components/DataTable.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import PullToRefresh from '@/components/PullToRefresh.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { buildRecordColumns, combineRecordFilter } from '@/components/records/recordColumns'
import { loadPageSize, savePageSize } from '@/tablePrefs'
import WorkspaceImage from '@/components/WorkspaceImage.vue'
import ArtPlaceholder from '../forms/ArtPlaceholder.vue'
import CheckboxField from '@/components/fields/CheckboxField.vue'
import DateField from '@/components/fields/DateField.vue'
import ImageField from '@/components/fields/ImageField.vue'
import LinkField from '@/components/fields/LinkField.vue'
import LongTextField from '@/components/fields/LongTextField.vue'
import NumberField from '@/components/fields/NumberField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import TextField from '@/components/fields/TextField.vue'
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker'
import { BookOpen, CalendarPlus, CircleAlert, Clock, Fingerprint, Hash, Layers, Pencil, Plus, Trash2, User } from '@/icons'
import { toast } from 'vue-sonner'

// The template reads the shared books-entry words through these aliases.
const outcomeLabel = postingOutcomeLabel
const amountText = ledgerAmount

// recordType is null when no saved Record Type exists yet. recordId comes from the page
// URL: '' for the table, 'new' for the create form, or an record ID.
// recordTypeKey is the key the address names; writes send it, and a record
// type that does not match it never saves.
const props = withDefaults(defineProps<{ recordType: RecordType | null; recordTypeKey: string; recordId: string; recordTypes?: RecordType[]; username?: string; canBuild?: boolean; canLedger?: boolean }>(), { username: '', canBuild: false, canLedger: false })
const emit = defineEmits<{ back: []; 'update:recordId': [id: string] }>()

const records = ref<RecordMessage[]>([])
const loading = ref(false)
const loadError = ref('')
// Server keyset paging: the API returns opaque forward tokens with no
// total, so the desk remembers the token that opened each page to walk
// back. Page size persists per user under the table id.
const page = ref(1)
const hasNext = ref(false)
const pageTokens = ref<string[]>([''])
const tableId = computed(() => props.recordType ? `records:${props.recordType.name}` : '')
const pageSize = ref(25)
watch(tableId, (id) => { pageSize.value = id ? loadPageSize(id, props.username) : 25 }, { immediate: true })
const serverPaging = computed(() => ({ page: page.value, total: null as number | null, hasNext: hasNext.value }))
const selectedIds = ref<string[]>([])
// Revisions seen on loaded pages, for bulk deletes of rows still in view;
// anything older is fetched before it is deleted.
const knownRevisions = new Map<string, bigint>()
const editing = ref<RecordMessage | 'new' | null>(null)
const texts = reactive<Record<string, string>>({})
const checks = reactive<Record<string, boolean>>({})
const images = reactive<Record<string, ImageFieldValue | null>>({})
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
const statusSelected = ref<string[]>([])
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

async function load(reset = true) {
  const recordType = props.recordType
  if (!recordType) { records.value = []; pageTokens.value = ['']; page.value = 1; hasNext.value = false; return }
  if (reset) { page.value = 1; pageTokens.value = [''] }
  const request = ++listRequest
  loading.value = true
  loadError.value = ''
  try {
    const res = await recordClient.listRecords({ recordTypeKey: recordType.name, pageSize: pageSize.value, pageToken: pageTokens.value[page.value - 1] ?? '', filter: appliedFilter.value })
    if (request !== listRequest) return
    records.value = res.records
    for (const item of res.records) knownRevisions.set(item.id, item.revision)
    hasNext.value = res.nextPageToken !== ''
    if (res.nextPageToken && page.value === pageTokens.value.length) pageTokens.value.push(res.nextPageToken)
  } catch (err) {
    if (request !== listRequest) return
    loadError.value = message(err)
    // A filter error shows inline only: the search applies while typing,
    // and a toast per half-written expression would nag.
    if (!appliedFilter.value) toast.error(`Could not load records: ${loadError.value}`, { action: { label: 'Retry', onClick: () => { void load(false) } } })
  } finally {
    if (request === listRequest) loading.value = false
  }
}

function prevPage() {
  if (page.value <= 1) return
  page.value -= 1
  void load(false)
}

function nextPage() {
  if (!hasNext.value) return
  page.value += 1
  void load(false)
}

function changePageSize(size: number) {
  pageSize.value = size
  if (tableId.value) savePageSize(tableId.value, props.username, size)
  void load(true)
}

watch(() => props.recordType?.name, () => {
  filterText.value = ''
  appliedFilter.value = ''
  statusSelected.value = []
  selectedIds.value = []
  knownRevisions.clear()
  void load(true)
}, { immediate: true })

// The search applies after a pause in typing; the Status filter applies at
// once. Both combine into the one server expression. The watcher skips a
// trailing call when the expression already applied (clearing filters, or
// switching Record Types, applies at once through the same path).
let filterTimer: ReturnType<typeof setTimeout> | undefined
function currentExpression(): string {
  return combineRecordFilter(filterText.value, statusSelected.value.map(Number).filter((state) => Number.isInteger(state)))
}
function applyListFilter() {
  appliedFilter.value = currentExpression()
  void load(true)
}
watch(filterText, () => {
  clearTimeout(filterTimer)
  if (currentExpression() === appliedFilter.value) return
  filterTimer = setTimeout(applyListFilter, 500)
})
function onFiltersChanged(next: TableFilter[]) {
  statusSelected.value = next.find((filter) => filter.id === 'status')?.selected ?? []
  clearTimeout(filterTimer)
  applyListFilter()
}
function onClearFilters() {
  clearTimeout(filterTimer)
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
  clear(texts); clear(checks); clear(images); clear(errors); clear(states); touched.clear()
  lastPreview = ''
  parentId.value = record === 'new' ? '' : record.parentId
  manualName.value = ''
  for (const field of props.recordType.fields) {
    const kind = record === 'new' ? undefined : record.values[field.name]?.kind
    // A new record shows the default the server will fill in.
    if (field.type === 'Checkbox') checks[field.name] = kind?.case === 'boolValue' ? kind.value : record === 'new' && field.default === 'true'
    else if (field.type === 'Image') images[field.name] = kind?.case === 'imageValue' ? { path: kind.value.path, hash: kind.value.hash, altText: kind.value.altText } : null
    else texts[field.name] = kind?.case === 'stringValue' ? kind.value : record === 'new' ? field.default ?? '' : ''
  }
  previewOpen.value = false
  booksOpen.value = false
  if (record !== 'new') void loadHistory(record)
  void loadPostingStatus()
  schedulePreview(0)
  void evaluateVisibility()
}

// Read-only fields take only their default, and a set-once field is fixed
// after it has a value. The server enforces both.
function locked(field: Field): boolean {
  if (field.read_only || isCalculated(field) || states[field.name]?.readOnly) return true
  if (!fieldEditable(field, submittable.value, docstate.value)) return true
  if (!field.set_once || editing.value === null || editing.value === 'new') return false
  const kind = editing.value.values[field.name]?.kind
  if (field.type === 'Image') return kind?.case === 'imageValue'
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

// Link cells show the linked Record's label, never its ID. Labels load once
// per page of Records and fill in as they arrive.
const linkLabels = ref<Record<string, string>>({})
watch(records, async (items) => {
  if (!props.recordType) return
  for (const field of props.recordType.fields) {
    if (field.type !== 'Link' || !field.target_record_type) continue
    const ids = items.flatMap((record) => { const kind = record.values[field.name]?.kind; return kind?.case === 'stringValue' ? [kind.value] : [] })
    if (ids.length === 0) continue
    try {
      const found = await resolveRecords(field.target_record_type, ids)
      linkLabels.value = { ...linkLabels.value, ...Object.fromEntries([...found].map(([id, item]) => [id, labelWithState(item)])) }
    } catch {
      // The cell stays empty; opening the Record shows the error toast if the lookup keeps failing.
    }
  }
}, { immediate: true })

const tableColumns = computed(() => props.recordType
  ? buildRecordColumns({
    fields: props.recordType.fields,
    submittable: submittable.value,
    linkLabels: linkLabels.value,
    primaryField: props.recordType.label_field,
    updatedLine: (item) => formatWhen(item.updatedAt),
  })
  : [])
const recordFilters = computed<TableFilter[]>(() => submittable.value
  ? [{ id: 'status', label: 'Status', options: [{ value: '0', label: gloss('draft').label }, { value: '1', label: gloss('submitted').label }, { value: '2', label: gloss('cancelled').label }], selected: statusSelected.value }]
  : [])

const bulkDeleting = ref(false)
const confirmingBulkDelete = ref(false)
const bulkActions = computed<ActionItem[]>(() => [
  { id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2', busy: bulkDeleting.value },
])
const pendingDelete = ref<RecordMessage | null>(null)
const confirmingSingleDelete = computed({
  get: () => pendingDelete.value !== null,
  set: (open: boolean) => { if (!open) pendingDelete.value = null },
})

function rowActions(target: RecordMessage): ActionItem[] {
  const settled = submittable.value && target.docstate !== docstates.draft
  return [
    { id: 'open', label: 'Open', kind: 'quiet', icon: 'eye' },
    {
      id: 'delete',
      label: 'Delete',
      kind: 'destructive',
      icon: 'trash-2',
      disabledReason: isSetting.value ? 'A setting is edited, not deleted.' : settled ? 'Only a draft can be deleted.' : null,
    },
  ]
}

function onRowAction(id: string, target: RecordMessage) {
  if (id === 'open') openRecord(target)
  else if (id === 'delete') pendingDelete.value = target
}

function onBulkAction(id: string) {
  if (id === 'delete') confirmingBulkDelete.value = true
}

// After a delete the page reloads; a page left empty steps back first.
function reloadAfterDelete() {
  if (records.value.length <= 1 && page.value > 1) page.value -= 1
  void load(false)
}

async function deleteOne(target: RecordMessage) {
  const recordType = props.recordType
  if (!recordType) return
  busy.value = true
  try {
    await recordClient.deleteRecord({ recordTypeKey: recordType.name, id: target.id, expectedRevision: target.revision })
    selectedIds.value = selectedIds.value.filter((id) => id !== target.id)
    knownRevisions.delete(target.id)
    forgetLabels(recordType.name)
    toast.success('Record deleted.')
    reloadAfterDelete()
  } catch (err) {
    reportWriteError(err)
  } finally {
    pendingDelete.value = null
    busy.value = false
  }
}

async function deleteSelected() {
  const recordType = props.recordType
  const ids = [...selectedIds.value]
  if (!recordType || ids.length === 0) return
  bulkDeleting.value = true
  let done = 0
  let failed = 0
  for (const id of ids) {
    try {
      let revision = knownRevisions.get(id)
      if (revision === undefined) {
        const res = await recordClient.getRecord({ recordTypeKey: recordType.name, id })
        revision = res.record?.revision
      }
      if (revision === undefined) throw new Error('Record not found.')
      await recordClient.deleteRecord({ recordTypeKey: recordType.name, id, expectedRevision: revision })
      knownRevisions.delete(id)
      done += 1
    } catch {
      failed += 1
    }
  }
  bulkDeleting.value = false
  confirmingBulkDelete.value = false
  selectedIds.value = []
  forgetLabels(recordType.name)
  if (failed === 0) toast.success(done === 1 ? 'Record deleted.' : `${done} records deleted.`)
  else if (done === 0) toast.error(failed === 1 ? 'Could not delete the record.' : `Could not delete ${failed} records.`)
  else toast.error(`Deleted ${done} of ${done + failed} records; the rest could not be deleted.`)
  void load(true)
}

function fieldLabel(field: Field): string {
  if (field.type === 'Money' && field.currency) return `${field.label} (${field.currency})`
  if (field.type === 'Quantity' && field.uom) return `${field.label} (${field.uom})`
  return field.label
}

// A field may be required by its required_if rule as well as by its own flag.
function effective(field: Field): Field {
  return states[field.name]?.required && !field.required ? { ...field, required: true } : field
}
// The form renders the layout's sections (B9); without a layout every field
// renders in definition order in one column.
const plannedSections = computed<PlannedSection[]>(() => planSections(props.recordType?.layout, props.recordType?.fields ?? []))
// Section visibility runs against the record being edited. New records and
// failures show the section: visibility only hides, so it must never hide on
// a doubt. Draft values do not re-evaluate it; saving re-opens the record
// and runs it again.
const hiddenSections = reactive<Record<number, boolean>>({})
let visibilityRequest = 0
const visibleSections = computed(() => plannedSections.value.filter((_, index) => !hiddenSections[index]))
async function evaluateVisibility() {
  const recordType = props.recordType
  const current = editing.value
  const request = ++visibilityRequest
  clear(hiddenSections)
  if (!recordType || !current || current === 'new') return
  await Promise.all(plannedSections.value.map(async (section, index) => {
    if (!section.visibility.trim()) return
    try {
      const res = await recordClient.evaluateExpression({ recordTypeKey: recordType.name, expression: section.visibility, id: current.id })
      if (request !== visibilityRequest || editing.value !== current) return
      if (res.evaluated && !res.isNull && res.value === 'false') hiddenSections[index] = true
    } catch {
      // A section that cannot be judged stays visible.
    }
  }))
}
// The template reads the shared span rule through this alias.
const spanClass = itemSpanClass
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
watch([texts, checks, images], () => schedulePreview(), { deep: true })
function check(field: Field) {
  const problem = field.type === 'Image'
    ? validateImageValue(effective(field), images[field.name] ?? null)
    : validateField(effective(field), texts[field.name] ?? '')
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
  if (field.type === 'Image') {
    const image = images[field.name] ?? null
    return create(FieldValueSchema, image ? { kind: { case: 'imageValue', value: { path: image.path, hash: image.hash, altText: image.altText } } } : { kind: { case: 'nullValue', value: true } })
  }
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
      await recordClient.createRecord({ recordTypeKey: props.recordTypeKey, values, parentId: isChild.value ? parentId.value.trim() : '', name: allowsManualName.value ? manualName.value.trim() : '' })
      forgetLabels(recordType.name)
      toast.success('Record created.')
      void load(true)
      close()
    } else {
      const current = editing.value
      const res = await recordClient.updateRecord({ recordTypeKey: props.recordTypeKey, id: current.id, expectedRevision: current.revision, values })
      const updated = res.record
      if (updated) open(updated)
      forgetLabels(recordType.name)
      toast.success('Record saved.')
      void load(false)
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
async function runTransition(action: Transition, approvedHash = '') {
  const recordType = props.recordType
  if (!editing.value || editing.value === 'new' || !recordType) return
  const current = editing.value
  // A submit from the preview dialog carries the approved plan hash, so a
  // plan that changed underneath is refused for review (R6.15).
  const request = { recordTypeKey: recordType.name, id: current.id, expectedRevision: current.revision, ...(action === 'submit' && approvedHash ? { postingPreviewHash: approvedHash } : {}) }
  busy.value = true
  try {
    const record = action === 'submit' ? (await recordClient.submitRecord(request)).record
      : action === 'cancel' ? (await recordClient.cancelRecord(request)).record
      : (await recordClient.amendRecord(request)).record
    if (record) {
      const fresh = record
      // An amend returns the new draft; the other two return the changed document.
      if (action === 'amend') {
        emit('update:recordId', fresh.id)
        void load(true)
      } else {
        open(fresh)
        void load(false)
      }
    }
    forgetLabels(recordType.name)
    toast.success(transitionText[action])
  } catch (err) {
    // A blocked books entry reads as a plain sentence; anything else keeps
    // its own error path. A stale preview refreshes itself so the new plan
    // is ready to review.
    const blocked = action === 'amend' ? null : postingBlockedMessage(err, action)
    if (blocked) {
      toast.error(blocked)
      if (previewOpen.value && err instanceof ConnectError && err.rawMessage.includes('changed since it was previewed')) void loadPreview()
    } else reportWriteError(err)
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
      open(res.record)
      toast.info('Record reloaded with the latest changes.')
      void load(false)
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
    knownRevisions.delete(current.id)
    selectedIds.value = selectedIds.value.filter((id) => id !== current.id)
    forgetLabels(recordType.name)
    toast.success('Record deleted.')
    reloadAfterDelete()
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

// Books entries (G6.10): what submitting recorded for a submittable
// document. The buttons stay hidden without ledger:read; the server checks
// that Grant and the document read Grant on every call.
const canSeePosting = computed(() => props.canLedger === true && submittable.value)
// One page of lines or attempts; the server default is 50.
const postingPageSize = 50

// Posting status: one row per posted group, loaded with the document so
// the badge beside the lifecycle state is there when the page settles. A
// failure hides the badge; the sheet shows the error with a retry.
const postingGroups = ref<PostingGroupStatus[]>([])
const postingStatusState = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
let statusRequest = 0
async function loadPostingStatus() {
  const recordType = props.recordType
  const current = editing.value
  if (!recordType || !current || current === 'new' || !canSeePosting.value || docstate.value === docstates.draft) {
    postingGroups.value = []
    postingStatusState.value = 'idle'
    return
  }
  const request = ++statusRequest
  postingStatusState.value = 'loading'
  try {
    const res = await ledgerClient.getPostingStatus({ recordTypeKey: recordType.name, id: current.id })
    if (request !== statusRequest) return
    postingGroups.value = res.groups
    postingStatusState.value = 'ready'
  } catch {
    if (request !== statusRequest) return
    postingGroups.value = []
    postingStatusState.value = 'error'
  }
}
// The badge beside the lifecycle state. A document whose submit groups all
// unwound reads Reversed; anything recorded reads Recorded in the books.
const booksBadge = computed<null | { status: string; label: string }>(() => {
  if (postingStatusState.value !== 'ready') return null
  const submits = postingGroups.value.filter((group) => group.event === 'submit')
  if (submits.length === 0) return null
  return submits.every((group) => group.reversed)
    ? { status: 'reversed', label: 'Reversed' }
    : { status: 'submitted', label: gloss('posted').label }
})
function groupTitle(group: PostingGroupStatus): string {
  const event = group.event === 'cancel' ? 'Reversal' : gloss('submitted').label
  const parts = [`Version ${group.revision}`, event]
  if (group.book) parts.push(`Book ${group.book}`)
  return parts.join(' · ')
}
function groupDetail(group: PostingGroupStatus): string {
  const lines = group.rowCount === BigInt(1) ? '1 line' : `${group.rowCount} lines`
  return `${lines} · ${when.format(new Date(group.postedAt))}`
}

// Preview: what submitting this draft would record (R6.14). The hash goes
// back with the submit, so a plan that changed underneath is refused for
// review instead of recording unseen lines (R6.15).
const previewOpen = ref(false)
const previewPlans = ref<PostingPlan[]>([])
const previewHash = ref('')
const previewState = ref<'idle' | 'loading' | 'ready' | 'error' | 'denied'>('idle')
const previewError = ref('')
async function openPreview() {
  previewOpen.value = true
  await loadPreview()
}
async function loadPreview() {
  const recordType = props.recordType
  const current = editing.value
  if (!recordType || !current || current === 'new') return
  previewState.value = 'loading'
  previewError.value = ''
  try {
    const res = await ledgerClient.previewPosting({ recordTypeKey: recordType.name, id: current.id })
    previewPlans.value = res.plans
    previewHash.value = res.planHash
    previewState.value = 'ready'
  } catch (err) {
    previewPlans.value = []
    previewHash.value = ''
    if (err instanceof ConnectError && err.code === Code.PermissionDenied) {
      previewState.value = 'denied'
    } else {
      previewState.value = 'error'
      previewError.value = postingPreviewBlocked(err) ?? message(err)
    }
  }
}
// One caption under a leg's account: dimensions, foreign amount and
// quantity, whichever the leg carries.
function legDetail(line: PostingPlanLine | LedgerRow): string {
  const parts: string[] = []
  const dimensions = Object.entries(line.dimensions ?? {}).map(([key, value]) => `${key}: ${value}`).join(' · ')
  if (dimensions) parts.push(dimensions)
  if (line.txnCurrency) {
    const txn = amountText(line.txnDebit) || amountText(line.txnCredit)
    if (txn) parts.push(`${txn} ${line.txnCurrency}`)
  }
  if (line.quantity) parts.push(line.quantityUom ? `Qty ${line.quantity} ${line.quantityUom}` : `Qty ${line.quantity}`)
  return parts.join(' · ')
}

// The Books entries sheet: posted lines and attempts for a submitted or
// cancelled document. Both lists page with a Show more button.
const booksOpen = ref(false)
const ledgerRows = ref<LedgerRow[]>([])
const ledgerNext = ref('')
const ledgerState = ref<'idle' | 'loading' | 'ready' | 'error' | 'denied'>('idle')
const ledgerError = ref('')
const ledgerMore = ref(false)
const logEntries = ref<PostingLogEntry[]>([])
const logNext = ref('')
const logState = ref<'idle' | 'loading' | 'ready' | 'error' | 'denied'>('idle')
const logError = ref('')
const logMore = ref(false)
async function openBooks() {
  booksOpen.value = true
  await Promise.all([loadLedgerRows(true), loadPostingLog(true)])
}
function denied(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.PermissionDenied
}
async function loadLedgerRows(first: boolean) {
  const recordType = props.recordType
  const current = editing.value
  if (!recordType || !current || current === 'new') return
  if (first) {
    ledgerState.value = 'loading'
    ledgerRows.value = []
    ledgerNext.value = ''
  } else {
    ledgerMore.value = true
  }
  ledgerError.value = ''
  try {
    const res = await ledgerClient.listLedgerRows({ recordTypeKey: recordType.name, id: current.id, pageSize: postingPageSize, pageToken: first ? '' : ledgerNext.value })
    ledgerRows.value = first ? res.rows : [...ledgerRows.value, ...res.rows]
    ledgerNext.value = res.nextPageToken
    ledgerState.value = 'ready'
  } catch (err) {
    if (denied(err)) ledgerState.value = 'denied'
    else {
      ledgerState.value = 'error'
      ledgerError.value = message(err)
    }
  } finally {
    ledgerMore.value = false
  }
}
async function loadPostingLog(first: boolean) {
  const recordType = props.recordType
  const current = editing.value
  if (!recordType || !current || current === 'new') return
  if (first) {
    logState.value = 'loading'
    logEntries.value = []
    logNext.value = ''
  } else {
    logMore.value = true
  }
  logError.value = ''
  try {
    const res = await ledgerClient.listPostingLog({ recordTypeKey: recordType.name, id: current.id, pageSize: postingPageSize, pageToken: first ? '' : logNext.value })
    logEntries.value = first ? res.entries : [...logEntries.value, ...res.entries]
    logNext.value = res.nextPageToken
    logState.value = 'ready'
  } catch (err) {
    if (denied(err)) logState.value = 'denied'
    else {
      logState.value = 'error'
      logError.value = message(err)
    }
  } finally {
    logMore.value = false
  }
}
// Nothing recorded anywhere: one quiet empty state instead of three.
const booksEmpty = computed(() => postingStatusState.value === 'ready'
  && ledgerState.value === 'ready' && logState.value === 'ready'
  && postingGroups.value.length === 0 && ledgerRows.value.length === 0 && logEntries.value.length === 0)
function attemptTitle(entry: PostingLogEntry): string {
  const event = entry.event === 'cancel' ? gloss('cancelled').label : gloss('submitted').label
  const parts = [outcomeLabel(entry.outcome), `Version ${entry.revision}`, event]
  if (entry.book) parts.push(`Book ${entry.book}`)
  return parts.join(' · ')
}
function attemptDetail(entry: PostingLogEntry): string {
  const said = entry.message ? `${entry.message} · ` : ''
  return `${said}${entry.actor} · ${when.format(new Date(entry.createdAt))}`
}
</script>

<template>
  <section class="page-content view-page">
    <div v-if="!recordType" class="empty-builder">
      <h2>Nothing to view yet</h2>
      <!-- View hides all Build navigation: only builders get the way back. -->
      <template v-if="canBuild">
        <p>Define the first form in Build, then add its records here.</p>
        <ActionButton kind="secondary" label="Go to Build" icon="hammer" @press="emit('back')" />
      </template>
      <p v-else>Lists appear here when someone shares them with you.</p>
    </div>

    <template v-else-if="editing">
      <button type="button" class="back-link" @click="close">← {{ recordType.label }}</button>
      <div class="page-title-row">
        <div class="entry-status">
          <h1>{{ editing === 'new' ? 'New record' : 'Edit record' }}</h1>
          <Badge v-if="submittable && isExisting" :variant="docstate === 2 ? 'destructive' : docstate === 1 ? 'default' : 'secondary'">{{ stateLabel }}</Badge>
          <StatusBadge v-if="booksBadge" :status="booksBadge.status" :label="booksBadge.label" />
          <small v-if="submittable && isExisting && editing !== 'new' && editing.name !== editing.id" class="muted-label">{{ editing.name }}</small>
        </div>
        <div v-if="submittable && editing !== 'new'" class="lifecycle-actions">
          <ActionButton v-if="docstate === 0 && canSeePosting" kind="secondary" label="Preview books entry" icon="eye" :busy="busy" @press="openPreview" />
          <ActionButton v-if="docstate === 0" kind="primary" label="Submit" icon="send" :busy="busy" @press="confirmingSubmit = true" />
          <ActionButton v-if="docstate !== 0 && canSeePosting" kind="secondary" label="Books entries" icon="book-open" @press="openBooks" />
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
      <ResponsiveDialog v-model:open="previewOpen" title="Preview the books entry" description="What submitting would record in the books. Nothing is recorded yet." wide>
        <div v-if="previewState === 'loading'" class="grid gap-2" aria-busy="true" aria-label="Loading preview">
          <Skeleton class="h-5 w-40 max-w-full" />
          <Skeleton class="h-16 w-full" />
          <Skeleton class="h-16 w-full" />
        </div>
        <div v-else-if="previewState === 'denied'" class="flex flex-col items-center gap-2 px-6 py-8 text-center">
          <CircleAlert class="size-5 text-muted-foreground" aria-hidden="true" />
          <p class="sheet-lead">You need permission to see the books.</p>
          <p class="muted-label">Ask an owner to grant it.</p>
        </div>
        <div v-else-if="previewState === 'error'" class="flex flex-col items-center gap-2 px-6 py-8 text-center">
          <CircleAlert class="size-5 text-destructive" aria-hidden="true" />
          <p>{{ previewError }}</p>
          <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="loadPreview" />
        </div>
        <div v-else-if="previewPlans.length === 0" class="px-1 py-4 text-center">
          <p class="sheet-lead">There is nothing to record for this document.</p>
          <p class="muted-label mt-1">Submitting it records no books entries.</p>
        </div>
        <div v-else class="grid gap-4">
          <div v-for="plan in previewPlans" :key="`${plan.book}:${plan.postingDate}`" class="grid gap-2">
            <p class="sheet-label"><span v-if="plan.book">Book {{ plan.book }} · </span>{{ plan.postingDate }}</p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Account</TableHead>
                  <TableHead class="text-right">Debit</TableHead>
                  <TableHead class="text-right">Credit</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow v-for="(line, index) in plan.lines" :key="index">
                  <TableCell>
                    <div class="font-medium">{{ line.accountLabel || line.account }}</div>
                    <div v-if="line.accountLabel" class="muted-label font-mono">{{ line.account }}</div>
                    <div v-if="legDetail(line)" class="muted-label">{{ legDetail(line) }}</div>
                  </TableCell>
                  <TableCell class="text-right tabular-nums">{{ amountText(line.debitBase) || '—' }}</TableCell>
                  <TableCell class="text-right tabular-nums">{{ amountText(line.creditBase) || '—' }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>
        <template #footer>
          <ActionButton kind="secondary" label="Close" icon="x" :busy="busy" @press="previewOpen = false" />
          <ActionButton v-if="previewState === 'ready' && previewPlans.length > 0" kind="primary" label="Submit document" icon="send" :busy="busy" @press="runTransition('submit', previewHash)" />
        </template>
      </ResponsiveDialog>
      <DetailSheet v-model:open="booksOpen" wide title="Books entries" description="What this document recorded in the books.">
        <template #status>
          <StatusBadge v-if="booksBadge" :status="booksBadge.status" :label="booksBadge.label" />
        </template>
        <div v-if="booksEmpty" class="px-1 py-4 text-center">
          <p class="sheet-lead">Nothing in the books yet.</p>
          <p class="muted-label mt-1">Submitting recorded no books entries for this document.</p>
        </div>
        <template v-else>
          <div>
            <p class="sheet-label">Status</p>
            <div v-if="postingStatusState === 'loading'" class="grid gap-2" aria-busy="true" aria-label="Loading status">
              <Skeleton class="h-12 w-full" />
            </div>
            <div v-else-if="postingStatusState === 'error'" class="flex flex-col items-start gap-2">
              <p class="muted-label">The status did not load.</p>
              <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="loadPostingStatus" />
            </div>
            <ol v-else-if="postingGroups.length > 0" class="entry-markers">
              <li v-for="group in postingGroups" :key="group.postingGroupId">
                <Marker variant="border"><MarkerIcon><BookOpen /></MarkerIcon><MarkerContent><span class="marker-label">{{ groupTitle(group) }}</span>{{ groupDetail(group) }}
                  <StatusBadge v-if="group.reversed" status="reversed" label="Reversed" />
                </MarkerContent></Marker>
              </li>
            </ol>
            <p v-else class="muted-label">Nothing recorded yet.</p>
          </div>
          <div>
            <p class="sheet-label">Lines</p>
            <div v-if="ledgerState === 'loading'" class="grid gap-2" aria-busy="true" aria-label="Loading lines">
              <Skeleton class="h-12 w-full" />
              <Skeleton class="h-12 w-full" />
            </div>
            <div v-else-if="ledgerState === 'denied'" class="flex flex-col items-center gap-2 px-6 py-8 text-center">
              <CircleAlert class="size-5 text-muted-foreground" aria-hidden="true" />
              <p class="sheet-lead">You need permission to see the books.</p>
              <p class="muted-label">Ask an owner to grant it.</p>
            </div>
            <div v-else-if="ledgerState === 'error'" class="flex flex-col items-center gap-2 px-6 py-8 text-center">
              <CircleAlert class="size-5 text-destructive" aria-hidden="true" />
              <p>{{ ledgerError }}</p>
              <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="loadLedgerRows(true)" />
            </div>
            <template v-else-if="ledgerRows.length > 0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Account</TableHead>
                    <TableHead class="text-right">Debit</TableHead>
                    <TableHead class="text-right">Credit</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-for="row in ledgerRows" :key="row.id">
                    <TableCell>
                      <div class="font-medium">{{ row.account }}</div>
                      <div class="muted-label">{{ row.postingDate }} · {{ row.period }}</div>
                      <div v-if="legDetail(row)" class="muted-label">{{ legDetail(row) }}</div>
                      <StatusBadge v-if="row.isReversal" status="reversed" label="Reversal" />
                    </TableCell>
                    <TableCell class="text-right tabular-nums">{{ amountText(row.debitBase) || '—' }}</TableCell>
                    <TableCell class="text-right tabular-nums">{{ amountText(row.creditBase) || '—' }}</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
              <div v-if="ledgerNext" class="mt-2">
                <ActionButton kind="secondary" label="Show more lines" icon="chevron-down" :busy="ledgerMore" @press="loadLedgerRows(false)" />
              </div>
            </template>
            <p v-else class="muted-label">No lines recorded.</p>
          </div>
          <div>
            <p class="sheet-label">Attempts</p>
            <div v-if="logState === 'loading'" class="grid gap-2" aria-busy="true" aria-label="Loading attempts">
              <Skeleton class="h-12 w-full" />
              <Skeleton class="h-12 w-full" />
            </div>
            <div v-else-if="logState === 'denied'" class="flex flex-col items-center gap-2 px-6 py-8 text-center">
              <CircleAlert class="size-5 text-muted-foreground" aria-hidden="true" />
              <p class="sheet-lead">You need permission to see the books.</p>
              <p class="muted-label">Ask an owner to grant it.</p>
            </div>
            <div v-else-if="logState === 'error'" class="flex flex-col items-center gap-2 px-6 py-8 text-center">
              <CircleAlert class="size-5 text-destructive" aria-hidden="true" />
              <p>{{ logError }}</p>
              <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="loadPostingLog(true)" />
            </div>
            <template v-else-if="logEntries.length > 0">
              <ol class="entry-markers">
                <li v-for="entry in logEntries" :key="entry.id">
                  <Marker variant="border"><MarkerIcon><Clock /></MarkerIcon><MarkerContent><span class="marker-label">{{ attemptTitle(entry) }}</span>{{ attemptDetail(entry) }}</MarkerContent></Marker>
                </li>
              </ol>
              <div v-if="logNext" class="mt-2">
                <ActionButton kind="secondary" label="Show more attempts" icon="chevron-down" :busy="logMore" @press="loadPostingLog(false)" />
              </div>
            </template>
            <p v-else class="muted-label">No attempts recorded.</p>
          </div>
        </template>
      </DetailSheet>
      <form ref="formRef" class="entry-form" novalidate @submit.prevent="submit">
        <div class="entry-form-sections">
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
          <section v-for="(section, sectionIndex) in visibleSections" :key="sectionIndex" class="layout-section" :aria-label="section.title || `Section ${sectionIndex + 1}`">
            <div v-if="section.title || section.description" class="layout-section-head">
              <h2 v-if="section.title">{{ section.title }}</h2>
              <p v-if="section.description" class="layout-section-description">{{ section.description }}</p>
            </div>
            <div class="layout-grid" :class="section.columns === 2 ? 'is-two' : 'is-one'">
              <template v-for="(item, itemIndex) in section.items" :key="itemIndex">
                <p v-if="item.kind === 'text'" class="layout-text is-wide">{{ item.text }}</p>
                <WorkspaceImage v-else-if="item.kind === 'image'" :path="item.path" alt="" class="is-wide layout-block-image" />
                <ArtPlaceholder v-else-if="item.kind === 'art'" class="is-wide" />
                <template v-else-if="item.kind === 'field' && !states[item.field.name]?.hidden">
                  <LinkField
                    v-if="item.field.type === 'Link' && item.field.target_record_type"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="texts[item.field.name]"
                    :record-type-key="item.field.target_record_type"
                    :target-label="targetLabel(item.field)"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                  <CheckboxField
                    v-else-if="item.field.type === 'Checkbox'"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="checks[item.field.name]"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                  />
                  <SelectField
                    v-else-if="item.field.type === 'Enum'"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="texts[item.field.name]"
                    :options="item.field.values ?? []"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                  <LongTextField
                    v-else-if="item.field.type === 'Long text'"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="texts[item.field.name]"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                  <DateField
                    v-else-if="item.field.type === 'Date'"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="texts[item.field.name]"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                  <NumberField
                    v-else-if="['Number', 'Currency', 'Money', 'Quantity'].includes(item.field.type)"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="texts[item.field.name]"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                  <ImageField
                    v-else-if="item.field.type === 'Image'"
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="images[item.field.name]"
                    :record-type-key="recordType.name"
                    :field-name="item.field.name"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                  <TextField
                    v-else
                    :id="`entry-${item.field.name}`"
                    :label="fieldLabel(item.field)"
                    :optional="!effective(item.field).required"
                    :help="item.helpText || undefined"
                    :error="errors[item.field.name] ?? calculatedError(item.field)"
                    v-model="texts[item.field.name]"
                    :type="inputType(item.field)"
                    :class="spanClass(section, item)"
                    :disabled="locked(item.field)"
                    @update:model-value="onInput(item.field)"
                    @blur="onBlur(item.field)"
                  />
                </template>
              </template>
            </div>
          </section>
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
      <PullToRefresh :refreshing="loading" @refresh="load(false)">
        <DataTable
          :key="recordType.name"
          :columns="tableColumns"
          :data="records"
          :table-id="tableId"
          :username="username"
          :get-row-id="(record) => record.id"
          v-model:selected-ids="selectedIds"
          :bulk-actions="bulkActions"
          :row-actions="rowActions"
          v-model:search="filterText"
          search-placeholder='Filter records, for example doc.total > dec("100")'
          :filters="recordFilters"
          :server-paging="serverPaging"
          :loading="loading"
          :error="loadError || null"
          :filtered="appliedFilter !== ''"
          clickable
          resizable
          :width-key="`athena-entry-columns:${recordType.name}`"
          empty-title="No records yet"
          empty-body="Create the first record to see it here."
          @row-click="openRecord"
          @row-action="onRowAction"
          @bulk-action="onBulkAction"
          @update:filters="onFiltersChanged"
          @clear-filters="onClearFilters"
          @prev-page="prevPage"
          @next-page="nextPage"
          @update:page-size="changePageSize"
          @retry="load(false)"
        >
          <template v-if="canCreate" #empty-action>
            <ActionButton kind="primary" label="Create the first record" icon="plus" @press="openRecord('new')" />
          </template>
        </DataTable>
      </PullToRefresh>
      <ConfirmDialog
        v-model:open="confirmingBulkDelete"
        :title="selectedIds.length === 1 ? 'Delete this record?' : `Delete ${selectedIds.length} records?`"
        body="They disappear from lists at once. This cannot be undone."
        :confirm-label="selectedIds.length === 1 ? 'Delete record' : `Delete ${selectedIds.length} records`"
        :busy="bulkDeleting"
        @confirm="deleteSelected"
      />
      <ConfirmDialog
        v-model:open="confirmingSingleDelete"
        title="Delete this record?"
        :body="recordType.kind === 'master' ? 'It disappears from lists. The row is kept, so its history stays.' : 'This cannot be undone.'"
        confirm-label="Delete record"
        :busy="busy"
        @confirm="pendingDelete && deleteOne(pendingDelete)"
      />
    </template>
  </section>
</template>
