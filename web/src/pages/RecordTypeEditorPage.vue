// The Record Type editor (§§19-20): one saved type per address, created
// through the name-first dialog. Header, tabs (Form placeholder, Fields,
// Settings, Rules, History) and the full editing behaviour moved unchanged
// from the old inline form.
<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { ConnectError } from '@connectrpc/connect'
import { ruleClient } from '@/api'
import type { Rule } from '@/gen/athena/v1/rules_pb'
import type { Field, Kind, RecordType } from '../types'
import { validateNamingSeries } from '../fieldRules'
import { formatWhen } from '../times'
import FieldSettingsDialog from '../components/FieldSettingsDialog.vue'
import ActionButton from '@/components/ActionButton.vue'
import HelpHint from '@/copy/HelpHint.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import EmptyState from '@/components/EmptyState.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import { snakeKey } from '@/components/record-types/recordTypeKeys'
import { Badge } from '@/components/ui/badge'
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { icon } from '@/icons'
import { recordTypeIcon } from '@/pages/registry'

const props = defineProps<{ recordTypes: RecordType[]; currentRecordType: RecordType; databaseState: string; typesLoaded: boolean; typesLoadFailed: boolean; editorKey: string }>()
const emit = defineEmits<{
  save: [complete: (saved: boolean) => void]
  delete: [recordType: RecordType]
  addField: []
  closeEditor: []
  viewRecords: [key: string]
  openRules: []
}>()

// The addressed type. An unknown key stays unknown once loading settles:
// no silent fallback.
const edited = computed(() => props.recordTypes.find((item) => item.name === props.editorKey) ?? null)
const unknown = computed(() => props.typesLoaded && !props.typesLoadFailed && !edited.value)
const waiting = computed(() => !edited.value && !unknown.value)

const tab = ref('fields')
const isPhone = useMediaQuery('(max-width: 639px)')

const newFields = reactive(new Set<Field>())
const manuallyNamedFields = new WeakSet<Field>()
const fieldKeys = new WeakMap<Field, number>()
let nextFieldKey = 0

function fieldKey(field: Field) {
  let key = fieldKeys.get(field)
  if (key === undefined) {
    key = nextFieldKey++
    fieldKeys.set(field, key)
  }
  return key
}

function fieldNameFromLabel(label: string, field: Field) {
  const taken = new Set(props.currentRecordType.fields.filter((item) => item !== field).map((item) => item.name.toLowerCase()))
  return snakeKey(label, 'field', taken, ['id', 'revision', 'created_at', 'updated_at', 'created_by', 'modified_by', 'owner', 'deleted_at', 'def_version', 'request_id', 'parent_id', 'idx'])
}
// The title edits in place (§20): click it, or Rename in the header menu.
// A blank commit keeps the old name; a rename saves with the next Save.
const titleEditing = ref(false)
const titleDraft = ref('')
const titleInput = ref<ComponentPublicInstance | null>(null)
function startTitleEdit() {
  titleDraft.value = props.currentRecordType.label
  titleEditing.value = true
  void nextTick(() => {
    const input = titleInput.value?.$el as HTMLInputElement | undefined
    input?.focus()
    input?.select()
  })
}
function commitTitleEdit() {
  if (!titleEditing.value) return
  titleEditing.value = false
  const next = titleDraft.value.trim()
  if (next) props.currentRecordType.label = next
}
function cancelTitleEdit() {
  titleEditing.value = false
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
  if (field.type === 'Long text' || field.type === 'Checkbox' || field.type === 'Image') { delete field.unique; delete field.index }
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
const fieldTypes = ['Text', 'Long text', 'Email', 'Phone', 'Number', 'Currency', 'Money', 'Quantity', 'Date', 'Select', 'Enum', 'Link', 'Checkbox', 'Image']

function save() {
  for (const field of props.currentRecordType.fields) normalizeNewFieldName(field)
  emit('save', (saved) => {
    if (saved) for (const field of props.currentRecordType.fields) newFields.delete(field)
  })
}

function isNewField(field: Field) {
  return newFields.has(field)
}

// Rules attached to the edited type, for the Rules tab.
const rules = ref<Rule[]>([])
const rulesState = ref<'loading' | 'ready' | 'error'>('loading')
async function loadRules() {
  rulesState.value = 'loading'
  try {
    rules.value = (await ruleClient.listRules({})).rules
    rulesState.value = 'ready'
  } catch {
    rulesState.value = 'error'
  }
}
onMounted(loadRules)
const attachedRules = computed(() => rules.value.filter((rule) => rule.definition?.recordTypeKey === props.currentRecordType.name))
const ruleTypeLabel = computed(() => {
  const labels: Record<string, string> = { validation: 'Validation', derivation: 'Derivation', guard: 'Guard', transform: 'Transform' }
  return (ruleType: string) => labels[ruleType] ?? ruleType
})
const kindBadge = computed(() => kinds.find((item) => item.value === props.currentRecordType.kind)?.label ?? props.currentRecordType.kind)
</script>

<template>
  <section class="page-content builder-page">
    <LoadingSkeleton v-if="waiting" />
    <EmptyState
      v-else-if="unknown"
      :icon="icon('file-x-2')"
      title="This Record Type no longer exists"
      body="It was renamed or deleted, or the address is mistyped."
    >
      <template #action>
        <ActionButton kind="primary" label="Back to Record Types" icon="arrow-left" @press="emit('closeEditor')" />
      </template>
    </EmptyState>
    <template v-else>
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink as-child>
              <button class="cursor-pointer" @click="emit('closeEditor')">Record Types</button>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{{ currentRecordType.label || 'Untitled' }}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
      <div class="page-title-row mt-2">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h1 v-if="!titleEditing" class="min-w-0 flex-1 cursor-pointer truncate" @click="startTitleEdit">{{ currentRecordType.label }}</h1>
            <Input v-else ref="titleInput" v-model="titleDraft" class="record-title-input min-w-0 flex-1" aria-label="Record Type name" @keyup.enter="commitTitleEdit" @keyup.escape="cancelTitleEdit" @blur="commitTitleEdit" />
            <ActionButton v-if="!titleEditing" kind="quiet" density="inline" icon-only label="Rename" icon="pencil" @press="startTitleEdit" />
            <Badge variant="secondary">{{ kindBadge }}</Badge>
            <span class="muted-label">Saved</span>
          </div>
        </div>
        <div class="max-[639px]:hidden">
          <div class="toolbar-actions">
            <ActionButton kind="secondary" density="row" label="View records" icon="eye" @press="emit('viewRecords', currentRecordType.name)" />
            <ActionButton kind="primary" density="row" label="Save" icon="save" @press="save" />
            <DropdownMenu>
              <DropdownMenuTrigger as-child>
                <Button variant="ghost" size="icon" aria-label="More actions">
                  <component :is="icon('ellipsis')" class="size-4" aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem @select="startTitleEdit">
                  <component :is="icon('pencil')" class="size-4" aria-hidden="true" />
                  <span>Rename</span>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem class="text-destructive focus:text-destructive" @select="confirmingTypeDelete = true">
                  <component :is="icon('trash-2')" class="size-4" aria-hidden="true" />
                  <span>Delete</span>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
      <Tabs v-model="tab" class="mt-4">
        <TabsList :variant="isPhone ? 'default' : 'line'" class="w-full min-[640px]:w-fit">
          <TabsTrigger value="form">Form</TabsTrigger>
          <TabsTrigger value="fields">Fields</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
          <TabsTrigger value="rules">Rules</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
        </TabsList>
        <TabsContent value="form">
          <EmptyState
            :icon="icon('layout-template')"
            title="Form builder coming soon"
            body="Design the layout of this form here: sections, columns, field order and help text. The Fields and Settings tabs work today."
          />
        </TabsContent>
        <TabsContent value="fields">
          <div class="overflow-x-auto"><div class="field-table min-w-[540px]"><div class="field-head"><span>Label</span><span>Key</span><span>Type</span><span>Required</span><span></span></div><div v-for="(field, index) in currentRecordType.fields" :key="fieldKey(field)" class="field-row"><span><Input :model-value="field.label" :aria-label="`Field ${index + 1} label`" @update:model-value="updateFieldLabel(field, $event)" /></span><span><Input :model-value="field.name" :readonly="!isNewField(field)" :aria-label="`Field ${index + 1} database name`" @update:model-value="updateFieldName(field, $event)" @blur="normalizeNewFieldName(field)" /></span><span><select v-model="field.type" aria-label="Field type" @change="changeType(field)"><option v-for="type in fieldTypes" :key="type">{{ type }}</option></select></span><span><input v-model="field.required" type="checkbox" :aria-label="`${field.label} required`" /></span><span class="field-actions"><ActionButton kind="quiet" density="inline" icon-only :label="`Settings for ${field.label}`" icon="sliders-horizontal" @press="settingsField = field" /><ActionButton kind="quiet" density="inline" icon-only :label="`Remove ${field.label}`" icon="x" @press="currentRecordType.fields.splice(index, 1)" /></span><div v-if="field.type === 'Link'" class="relationship-editor"><label>Links to<select v-model="field.target_record_type"><option :value="undefined" disabled>Select a Record Type</option><option v-for="target in recordTypes.filter((item) => item.name !== currentRecordType.name)" :key="target.name" :value="target.name">{{ target.label }}</option></select></label></div></div></div></div>
          <ActionButton kind="quiet" label="Add field" icon="plus" @press="addField" />
        </TabsContent>
        <TabsContent value="settings">
          <div class="record-meta">
            <label>Key<Input :model-value="currentRecordType.name" readonly aria-label="Record Type name" /><small>Fixed after the first save.</small></label>
            <label>Description<Input v-model="currentRecordType.description" placeholder="Optional" aria-label="Description" /></label>
          </div>
          <div class="record-meta record-kind">
            <label>Kind <HelpHint term="recordKind" /><select :value="currentRecordType.kind" disabled aria-label="Record Type kind" @change="chooseKind(($event.target as HTMLSelectElement).value as Kind)"><option v-for="kind in kinds" :key="kind.value" :value="kind.value">{{ kind.label }}</option></select><small>Fixed after the first save. {{ kindHint }}</small></label>
            <label v-if="currentRecordType.kind === 'document'">Class<select v-model="currentRecordType.class" aria-label="Document class" @change="chooseClass"><option v-for="item in documentClasses" :key="item[0]" :value="item[0]">{{ item[1] }}</option></select><small>Posting documents will write to ledgers; work and batch documents track work.</small></label>
            <label v-if="currentRecordType.kind === 'document' && currentRecordType.class !== 'work'" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.submittable ?? false" aria-label="Submittable" @update:model-value="setSubmittable($event === true)" />Submittable</span><small>Records move from draft to submitted to cancelled. Submitted records stay fixed, apart from fields set to allow editing.</small></label>
            <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.amendable ?? false" aria-label="Amendable" @update:model-value="setAmendable($event === true)" />Amendable</span><small>A cancelled record can be amended into a new draft that points back to it.</small></label>
            <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable" :class="{ 'is-invalid': namingProblem }">Naming series<Input :model-value="currentRecordType.naming_series ?? ''" placeholder="INV-{yyyy}-{#####}" aria-label="Naming series" :aria-invalid="namingProblem ? 'true' : undefined" @update:model-value="setNamingSeries(String($event))" /><small>{{ namingProblem || 'Numbers each document when it is submitted. Tokens: {yyyy}, {yy}, {mm} and one counter such as {#####}. Empty keeps the Record ID as the name.' }}</small></label>
            <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable && currentRecordType.naming_series" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.gap_free ?? false" aria-label="Gap-free numbering" @update:model-value="setFlag('gap_free', $event === true)" />Gap-free numbering</span><small>A submit that fails returns its number, so the series has no gaps. Submits take turns. Without it numbers can skip.</small></label>
            <label v-if="currentRecordType.kind === 'document' && currentRecordType.submittable" class="switch-label"><span class="switch-line"><Checkbox :model-value="currentRecordType.allow_manual_name ?? false" aria-label="Allow manual names" @update:model-value="setFlag('allow_manual_name', $event === true)" />Allow manual names</span><small>A new document can take a name typed by hand, for example when moving old documents in. It must be unused and is logged.</small></label>
            <label>Label field<select :value="currentRecordType.label_field ?? ''" aria-label="Label field" @change="setLabelField(($event.target as HTMLSelectElement).value)"><option value="">Automatic</option><option v-for="field in labelFieldChoices" :key="field.name" :value="field.name">{{ field.label }}</option></select><small>The field that labels a Record when it is chosen from a list. Automatic uses the record name, then the first required text field.</small></label>
            <label v-if="currentRecordType.kind === 'child'">Belongs to<select v-model="currentRecordType.parent_record_type" disabled aria-label="Parent Record Type"><option :value="undefined" disabled>Select a saved Record Type</option><option v-for="item in parentChoices" :key="item.name" :value="item.name">{{ item.label }}</option></select><small>Fixed after the first save.</small></label>
          </div>
          <p v-if="databaseState !== 'Connected'" class="muted-label">The database is not connected. Changes stay in this browser session.</p>
        </TabsContent>
        <TabsContent value="rules">
          <div class="mb-3 flex items-center justify-between gap-2">
            <p class="text-muted-foreground text-sm">Rules that check or change this Record Type's records.</p>
            <ActionButton kind="secondary" density="row" label="Open Rules" icon="scroll-text" @press="emit('openRules')" />
          </div>
          <LoadingSkeleton v-if="rulesState === 'loading'" />
          <div v-else-if="rulesState === 'error'" class="flex flex-wrap items-center gap-2">
            <p class="text-sm">The Rules did not load.</p>
            <ActionButton kind="secondary" density="row" label="Try again" icon="refresh-ccw" @press="loadRules" />
          </div>
          <p v-else-if="attachedRules.length === 0" class="text-muted-foreground text-sm">No Rules use this Record Type yet.</p>
          <ul v-else class="space-y-2">
            <li v-for="rule in attachedRules" :key="rule.definition?.key" class="flex flex-wrap items-center gap-2 rounded-lg border p-3">
              <component :is="recordTypeIcon" class="text-muted-foreground size-4" aria-hidden="true" />
              <span class="font-medium">{{ rule.definition?.label || rule.definition?.key }}</span>
              <Badge variant="secondary">{{ ruleTypeLabel(rule.definition?.ruleType ?? '') }}</Badge>
              <span class="text-muted-foreground text-sm">{{ rule.definition?.event }}</span>
              <span v-if="rule.definition && !rule.definition.enabled" class="text-muted-foreground text-sm">Disabled</span>
            </li>
          </ul>
        </TabsContent>
        <TabsContent value="history">
          <dl class="max-w-xl space-y-2 text-sm">
            <div class="flex items-center justify-between gap-4">
              <dt class="text-muted-foreground">Last saved</dt>
              <dd>{{ currentRecordType.updated_at ? formatWhen(currentRecordType.updated_at) : 'Unknown' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-4">
              <dt class="text-muted-foreground">Changed by</dt>
              <dd>{{ currentRecordType.updated_by || 'Unknown' }}</dd>
            </div>
          </dl>
          <p class="text-muted-foreground mt-4 text-sm">Earlier saves live in the journal. Full save history needs a journal API that is not built yet.</p>
        </TabsContent>
      </Tabs>
      <div aria-hidden="true" class="h-44 min-[640px]:hidden" />
      <div class="bg-card fixed inset-x-0 z-30 border-t px-4 pt-3 min-[640px]:hidden" style="bottom: calc(var(--space-16) + var(--space-3) + env(safe-area-inset-bottom, 0px) + var(--space-2)); padding-bottom: var(--space-3)">
        <div class="flex gap-2">
          <ActionButton kind="secondary" density="touch" label="View records" icon="eye" full-bleed @press="emit('viewRecords', currentRecordType.name)" />
          <ActionButton kind="primary" density="touch" label="Save" icon="save" full-bleed @press="save" />
        </div>
        <Separator class="my-2" />
        <ActionButton kind="destructive" density="touch" label="Delete" icon="trash-2" full-bleed @press="confirmingTypeDelete = true" />
      </div>
      <ConfirmDialog
        v-model:open="confirmingTypeDelete"
        :title="`Delete ${currentRecordType.label}?`"
        body="Its table and every record in it are deleted. This cannot be undone."
        confirm-label="Delete Record Type"
        confirm-icon="trash-2"
        danger
        @confirm="confirmingTypeDelete = false; emit('delete', currentRecordType)"
      />
      <FieldSettingsDialog :field="settingsField" :lifecycle="(currentRecordType.kind === 'document' && currentRecordType.submittable === true) || currentRecordType.kind === 'child'" @close="settingsField = null" />
    </template>
  </section>
</template>
