<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ruleClient } from '../api'
import type { Rule, RuleRun, RuleVersion, SimulateRuleResponse } from '../gen/athena/v1/rules_pb'
import { isRuleType, ruleEvents, ruleProblem, ruleTypeHelp, ruleTypes, starterSource, type RuleType } from '../ruleDefinitions'
import type { RecordType } from '../types'
import ActionButton from '@/components/ActionButton.vue'
import HelpHint from '@/copy/HelpHint.vue'
import ConditionBuilder from '@/components/conditions/ConditionBuilder.vue'
import { compileTree, type BuilderField, type ConditionTree, type TreeField } from '@/conditions'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type TableFilter } from '@/components/DataTable.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import PrimaryCell from '@/components/PrimaryCell.vue'
import { formatWhen } from '../times'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'

// The Rules page edits Starlark Rules through RuleService. The server compiles
// every source on save and returns the line of a mistake; ruleDefinitions.ts
// only repeats the definition rules so a slip shows early. Starlark Rules are
// not production-ready (ADR-0016, Proposed).
const props = withDefaults(defineProps<{ canWrite: boolean; recordTypes: RecordType[]; username?: string }>(), { username: '' })

type RuleSet = { field: string; valueExpr: string }
type Form = { key: string; label: string; ruleType: string; recordTypeKey: string; event: string; priority: number; enabled: boolean; source: string; mode: 'builder' | 'starlark'; tree: ConditionTree | null; message: string; sets: RuleSet[] }
const blank = (): Form => ({ key: '', label: '', ruleType: 'validation', recordTypeKey: '', event: 'validate', priority: 100, enabled: true, source: starterSource.validation, mode: 'builder', tree: null, message: '', sets: [] })

const rules = ref<Rule[]>([])
const loaded = ref(false)
const unavailable = ref('')
const selected = ref('')
const creating = ref(false)
const form = ref<Form>(blank())
const saved = ref<Form>(blank())
const version = ref(0)
const busy = ref(false)
const detail = ref<'editor' | 'runs' | 'versions'>('editor')
const runs = ref<RuleRun[]>([])
const versions = ref<RuleVersion[]>([])
const previousType = ref<RuleType>('validation')

const documentTypes = computed(() => props.recordTypes.filter((item) => item.kind === 'document' && item.revision > 0))
const events = computed(() => (isRuleType(form.value.ruleType) ? ruleEvents[form.value.ruleType] : []))
// Builder authoring: the selected type's fields as builder fields plus the
// compile map, and the authoring object ruleProblem validates.
const selectedType = computed(() => props.recordTypes.find((item) => item.name === form.value.recordTypeKey) ?? null)
function fieldKind(type: string): TreeField['kind'] {
  if (type === 'Number' || type === 'Currency' || type === 'Quantity') return 'decimal'
  if (type === 'Money') return 'money'
  if (type === 'Date') return 'date'
  if (type === 'Checkbox') return 'bool'
  return 'string'
}
const builderFields = computed<BuilderField[]>(() => (selectedType.value?.fields ?? []).map((f) => ({
  name: `doc.${f.name}`,
  label: f.label,
  type: f.type,
  kind: fieldKind(f.type),
  currency: f.currency,
  scale: f.scale,
  choices: f.values?.map((c) => c.value),
})))
const builderVars = computed<Record<string, TreeField>>(() => {
  const vars: Record<string, TreeField> = {}
  for (const f of builderFields.value) vars[f.name] = { kind: f.kind, currency: f.currency, scale: f.scale }
  return vars
})
const builderOffered = computed(() => ['validation', 'derivation', 'guard'].includes(form.value.ruleType))
const problem = computed(() => {
  if (!selected.value && !creating.value) return null
  if (form.value.mode === 'builder' && builderOffered.value) {
    return ruleProblem(form.value, { mode: 'builder', tree: form.value.tree, message: form.value.message, sets: form.value.sets, fields: builderVars.value })
  }
  return ruleProblem(form.value)
})
const dirty = computed(() => (selected.value !== '' || creating.value) && JSON.stringify(form.value) !== JSON.stringify(saved.value))
const isOpen = computed(() => selected.value !== '' || creating.value)

function describe(err: unknown, action: string): string {
  if (!(err instanceof ConnectError)) return `Could not ${action}.`
  if (err.code === Code.PermissionDenied) return `You do not have permission to ${action}.`
  if (err.code === Code.Aborted) return 'This Rule changed since you opened it. Reload it, then apply your edit again.'
  return `Could not ${action}: ${err.rawMessage}`
}

function toForm(rule: Rule): Form {
  const d = rule.definition
  const base = { key: d?.key ?? '', label: d?.label ?? '', ruleType: d?.ruleType ?? 'validation', recordTypeKey: d?.recordTypeKey ?? '', event: d?.event ?? '', priority: d?.priority ?? 100, enabled: d?.enabled ?? true, source: d?.source ?? '' }
  // A stored compiled expression means builder-authored; anything else
  // opens as Starlark.
  if (d?.conditionExpr) {
    try {
      const tree = JSON.parse(d.conditionTree) as ConditionTree
      return { ...base, mode: 'builder', tree, message: d.message ?? '', sets: (d.sets ?? []).map((s) => ({ field: s.field, valueExpr: s.valueExpr })) }
    } catch {
      // Corrupt tree JSON falls back to Starlark, where the empty source
      // prompts a fix instead of rendering a broken builder.
    }
  }
  return { ...base, mode: 'starlark', tree: null, message: '', sets: [] }
}

async function refresh() {
  try {
    rules.value = (await ruleClient.listRules({})).rules
    unavailable.value = ''
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.Unavailable) unavailable.value = 'The Athena server is unavailable. Check that it is running, then try again.'
    else toast.error(describe(err, 'load the Rules'))
  } finally {
    loaded.value = true
  }
}

function open(rule: Rule) {
  form.value = toForm(rule)
  saved.value = toForm(rule)
  previousType.value = isRuleType(form.value.ruleType) ? form.value.ruleType : 'validation'
  version.value = Number(rule.version)
  selected.value = rule.definition?.key ?? ''
  creating.value = false
  detail.value = 'editor'
  selectedRun.value = null
  runs.value = []
  versions.value = []
}

// Moving away from unsaved edits asks first.
const pending = ref<null | (() => void)>(null)
function guarded(action: () => void) {
  if (dirty.value) pending.value = action
  else action()
}
function choose(rule: Rule) {
  if (rule.definition?.key === selected.value && !creating.value) return
  guarded(() => open(rule))
}
function startNew() {
  guarded(() => {
    form.value = blank()
    saved.value = blank()
    previousType.value = 'validation'
    version.value = 0
    selected.value = ''
    creating.value = true
    detail.value = 'editor'
    selectedRun.value = null
  })
}
function proceed() {
  const action = pending.value
  pending.value = null
  action?.()
}

function addSet() {
  const first = builderFields.value[0]?.name ?? ''
  form.value = { ...form.value, sets: [...form.value.sets, { field: first, valueExpr: '' }] }
}

function updateSet(index: number, patch: Partial<RuleSet>) {
  const sets = form.value.sets.map((s, i) => (i === index ? { ...s, ...patch } : s))
  form.value = { ...form.value, sets }
}

function removeSet(index: number) {
  form.value = { ...form.value, sets: form.value.sets.filter((_, i) => i !== index) }
}

// Changing the type resets what depends on it.
watch(() => form.value.ruleType, (type) => {
  if (!isRuleType(type)) return
  const allowed = ruleEvents[type]
  if (!allowed.includes(form.value.event)) form.value.event = allowed[0] ?? ''
  if (type === 'transform') {
    form.value.recordTypeKey = ''
    form.value.mode = 'starlark'
  }
  if (creating.value && form.value.source === starterSource[previousType.value]) form.value.source = starterSource[type]
  previousType.value = type
})

// Builder and Starlark authoring are mutually exclusive on save: each
// clears the other's fields, so the server never sees both. The server
// compiles the tree; the Desk sends no expression.
function toDefinition() {
  const builder = form.value.mode === 'builder' && builderOffered.value
  return {
    key: form.value.key,
    label: form.value.label,
    ruleType: form.value.ruleType,
    recordTypeKey: form.value.recordTypeKey,
    event: form.value.event,
    priority: form.value.priority,
    enabled: form.value.enabled,
    source: builder ? '' : form.value.source,
    conditionTree: builder && form.value.tree ? JSON.stringify(form.value.tree) : '',
    conditionExpr: '',
    message: builder ? form.value.message : '',
    sets: builder ? form.value.sets.map((s) => ({ field: s.field, valueExpr: s.valueExpr })) : [],
  }
}

async function save() {
  if (!props.canWrite || !dirty.value || problem.value) return
  busy.value = true
  try {
    const res = await ruleClient.saveRule({ definition: toDefinition(), expectedVersion: BigInt(version.value) })
    toast.success(`Saved ${form.value.key} as version ${res.rule?.version}.`)
    await refresh()
    if (res.rule) open(res.rule)
  } catch (err) {
    toast.error(describe(err, 'save the Rule'))
  } finally {
    busy.value = false
  }
}

async function loadRuns() {
  const key = selected.value
  if (!key) return
  try {
    const res = await ruleClient.listRuleRuns({ ruleKey: key, pageSize: 50 })
    if (selected.value === key) runs.value = res.runs
  } catch (err) {
    if (selected.value === key) toast.error(describe(err, 'load the runs'))
  }
}
async function loadVersions() {
  const key = selected.value
  if (!key) return
  try {
    const res = await ruleClient.listRuleVersions({ key })
    if (selected.value === key) versions.value = res.versions
  } catch (err) {
    if (selected.value === key) toast.error(describe(err, 'load the versions'))
  }
}
// The §16 run sheet: tapping a run row shows the outcome, the messages it
// reported and the input hash. Rows carry everything, so no fetch races.
const selectedRun = ref<RuleRun | null>(null)
function openRun(run: RuleRun) {
  selectedRun.value = selectedRun.value?.id === run.id ? null : run
}

function onDetail(value: string | number) {
  detail.value = value === 'runs' ? 'runs' : value === 'versions' ? 'versions' : 'editor'
  selectedRun.value = null
  if (detail.value === 'runs') void loadRuns()
  if (detail.value === 'versions') void loadVersions()
}
function loadVersion(item: RuleVersion) {
  const d = item.definition
  if (!d) return
  const loaded = toForm({ definition: d } as Rule)
  form.value = { ...form.value, label: loaded.label, ruleType: loaded.ruleType, recordTypeKey: loaded.recordTypeKey, event: loaded.event, priority: loaded.priority, enabled: loaded.enabled, source: loaded.source, mode: loaded.mode, tree: loaded.tree, message: loaded.message, sets: loaded.sets }
  detail.value = 'editor'
  toast.success(`Version ${item.version} is in the editor. Save to make it the current version.`)
}

// Switching authoring modes confirms first: Builder → Starlark renders the
// condition as a commented scaffold; Starlark → Builder starts over blank.
const switchTo = ref<'builder' | 'starlark' | null>(null)
function askSwitch(mode: 'builder' | 'starlark') {
  if (mode === form.value.mode) return
  switchTo.value = mode
}
function confirmSwitch() {
  const mode = switchTo.value
  switchTo.value = null
  if (!mode || mode === form.value.mode) return
  if (mode === 'starlark') {
    const lines = ['# Converted from the builder. Keep the condition below or delete it and write Starlark.']
    if (form.value.tree) {
      try {
        const vars: Record<string, TreeField> = {}
        for (const f of builderFields.value) vars[f.name] = { kind: f.kind, currency: f.currency, scale: f.scale }
        lines.push(`# Condition was: ${compileTree(form.value.tree, vars)}`)
      } catch {
        lines.push('# Condition was: (does not compile)')
      }
    }
    if (form.value.message) lines.push(`# Message was: ${form.value.message}`)
    for (const set of form.value.sets) lines.push(`# Set was: ${set.field} = ${set.valueExpr}`)
    const starter = isRuleType(form.value.ruleType) ? starterSource[form.value.ruleType] : ''
    form.value = { ...form.value, mode: 'starlark', source: `${lines.join('\n')}\n\n${starter}` }
  } else {
    form.value = { ...form.value, mode: 'builder', tree: null, message: '', sets: [] }
  }
}

const deleting = ref(false)
const deletingRule = ref<{ key: string; version: bigint } | null>(null)
async function removeRule(key: string, version: bigint) {
  busy.value = true
  try {
    await ruleClient.deleteRule({ key, expectedVersion: version })
    toast.success(`Deleted ${key}. Its versions and runs are kept.`)
    if (selected.value === key) {
      selected.value = ''
      form.value = blank()
      saved.value = blank()
    }
    await refresh()
  } catch (err) {
    toast.error(describe(err, 'delete the Rule'))
  } finally {
    busy.value = false
  }
}
async function remove() {
  const target = deletingRule.value ?? { key: selected.value, version: BigInt(version.value) }
  deletingRule.value = null
  deleting.value = false
  if (!target.key) return
  await removeRule(target.key, target.version)
}

function askDeleteSelected() {
  deletingRule.value = null
  deleting.value = true
}

function onRuleRowAction(id: string, rule: Rule) {
  if (id !== 'delete') return
  deletingRule.value = { key: rule.definition?.key ?? '', version: rule.version }
  deleting.value = true
}

// Rules table: search across keys and labels, Type and Status filters,
// and the editor's Delete in the row menu.
const ruleSearch = ref('')
const ruleTypeSelected = ref<string[]>([])
const ruleEnabledSelected = ref<string[]>([])
const ruleFilters = computed<TableFilter[]>(() => [
  { id: 'type', label: 'Type', options: ruleTypes.map((type) => ({ value: type, label: type })), selected: ruleTypeSelected.value },
  { id: 'status', label: 'Status', options: [{ value: 'on', label: 'Enabled' }, { value: 'off', label: 'Disabled' }], selected: ruleEnabledSelected.value },
])
const ruleFilteredActive = computed(() => ruleSearch.value.trim() !== '' || ruleTypeSelected.value.length > 0 || ruleEnabledSelected.value.length > 0)
const filteredRules = computed(() => {
  const needle = ruleSearch.value.trim().toLowerCase()
  return rules.value.filter((rule) => {
    const d = rule.definition
    if (ruleTypeSelected.value.length > 0 && !ruleTypeSelected.value.includes(d?.ruleType ?? '')) return false
    if (ruleEnabledSelected.value.length > 0 && !ruleEnabledSelected.value.includes(d?.enabled ? 'on' : 'off')) return false
    return !needle || `${d?.key ?? ''} ${d?.label ?? ''}`.toLowerCase().includes(needle)
  })
})
function onRuleFiltersChanged(next: TableFilter[]) {
  ruleTypeSelected.value = next.find((filter) => filter.id === 'type')?.selected ?? []
  ruleEnabledSelected.value = next.find((filter) => filter.id === 'status')?.selected ?? []
}

const ruleColumns = [
  {
    id: 'rule', header: 'Rule', accessorFn: (rule: Rule) => (rule.definition?.key ?? '').toLowerCase(), meta: { label: 'Rule' },
    cell: ({ row }: { row: { original: Rule } }) => h(PrimaryCell, { title: row.original.definition?.key ?? '', subtitle: row.original.definition?.label || null }),
  },
  {
    id: 'type', header: 'Type', accessorFn: (rule: Rule) => rule.definition?.ruleType ?? '', meta: { label: 'Type' },
    cell: ({ row }: { row: { original: Rule } }) => row.original.definition?.ruleType ?? '',
  },
  {
    id: 'recordType', header: 'Record Type', accessorFn: (rule: Rule) => rule.definition?.recordTypeKey ?? '', meta: { label: 'Record Type', hideOnMobile: true },
    cell: ({ row }: { row: { original: Rule } }) => {
      const key = row.original.definition?.recordTypeKey ?? ''
      return key === '' ? h('span', { class: 'text-muted-foreground' }, '—') : key
    },
  },
  {
    id: 'event', header: 'Event', accessorFn: (rule: Rule) => rule.definition?.event ?? '', meta: { label: 'Event', hideOnMobile: true },
    cell: ({ row }: { row: { original: Rule } }) => row.original.definition?.event || 'called',
  },
  {
    id: 'priority', header: 'Priority', accessorFn: (rule: Rule) => rule.definition?.priority ?? 0, meta: { label: 'Priority', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: Rule } }) => String(row.original.definition?.priority ?? 0),
  },
  {
    id: 'enabled', header: 'Enabled', accessorFn: (rule: Rule) => (rule.definition?.enabled ? 1 : 0), meta: { label: 'Enabled', center: true },
    cell: ({ row }: { row: { original: Rule } }) => (row.original.definition?.enabled ? 'Yes' : 'No'),
  },
]

// Runs table: the loaded runs (the API pages beyond 50, which the desk
// leaves alone, as before) with Outcome and Event filters.
const runSearch = ref('')
const runOutcomeSelected = ref<string[]>([])
const runEventSelected = ref<string[]>([])
const runFilters = computed<TableFilter[]>(() => [
  { id: 'outcome', label: 'Outcome', options: ['ok', 'rejected', 'error'].map((outcome) => ({ value: outcome, label: outcome })), selected: runOutcomeSelected.value },
  {
    id: 'event', label: 'Event',
    options: [...new Set(runs.value.map((run) => run.event))].sort().map((event) => ({ value: event, label: event || 'called' })),
    selected: runEventSelected.value,
  },
])
const runFilteredActive = computed(() => runSearch.value.trim() !== '' || runOutcomeSelected.value.length > 0 || runEventSelected.value.length > 0)
const filteredRuns = computed(() => {
  const needle = runSearch.value.trim().toLowerCase()
  return runs.value.filter((run) => {
    if (runOutcomeSelected.value.length > 0 && !runOutcomeSelected.value.includes(run.outcome)) return false
    if (runEventSelected.value.length > 0 && !runEventSelected.value.includes(run.event)) return false
    return !needle || `${run.actor} ${run.code} ${run.message} ${run.event}`.toLowerCase().includes(needle)
  })
})
function onRunFiltersChanged(next: TableFilter[]) {
  runOutcomeSelected.value = next.find((filter) => filter.id === 'outcome')?.selected ?? []
  runEventSelected.value = next.find((filter) => filter.id === 'event')?.selected ?? []
}

const runColumns = [
  {
    id: 'when', header: 'When', accessorFn: (run: RuleRun) => run.createdAt, meta: { label: 'When' },
    cell: ({ row }: { row: { original: RuleRun } }) => h(PrimaryCell, { title: formatWhen(row.original.createdAt), subtitle: row.original.actor || null }),
  },
  {
    id: 'version', header: 'Version', accessorFn: (run: RuleRun) => Number(run.ruleVersion), meta: { label: 'Version', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: RuleRun } }) => `v${row.original.ruleVersion}`,
  },
  {
    id: 'event', header: 'Event', accessorFn: (run: RuleRun) => run.event, meta: { label: 'Event' },
    cell: ({ row }: { row: { original: RuleRun } }) => {
      const run = row.original
      return run.dryRun ? h('span', null, [run.event || 'called', h('small', { class: 'files-sub' }, ' dry run')]) : (run.event || 'called')
    },
  },
  {
    id: 'outcome', header: 'Outcome', accessorFn: (run: RuleRun) => run.outcome, meta: { label: 'Outcome' },
    cell: ({ row }: { row: { original: RuleRun } }) => {
      const run = row.original
      const parts: Array<ReturnType<typeof h>> = [h(Badge, { variant: run.outcome === 'ok' ? 'secondary' : 'outline' }, () => run.outcome)]
      if (run.code) parts.push(h('small', { class: 'files-sub' }, ` ${run.code}`))
      return h('span', { class: 'inline-flex items-center gap-1' }, parts)
    },
  },
  {
    id: 'steps', header: 'Steps', accessorFn: (run: RuleRun) => Number(run.steps), meta: { label: 'Steps', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: RuleRun } }) => String(row.original.steps),
  },
  {
    id: 'duration', header: 'Duration', accessorFn: (run: RuleRun) => Number(run.durationMicros), meta: { label: 'Duration', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: RuleRun } }) => microseconds(row.original.durationMicros),
  },
  {
    id: 'input', header: 'Input', accessorFn: (run: RuleRun) => run.inputHash, meta: { label: 'Input', hideOnMobile: true },
    cell: ({ row }: { row: { original: RuleRun } }) => h('code', null, row.original.inputHash.slice(0, 10)),
  },
]

// Simulate: a rolled-back dry run of the edited source.
const simulating = ref(false)
const simulation = ref<SimulateRuleResponse | null>(null)
const recordId = ref('')
const valueLines = ref('')
const simulateDirty = computed(() => simulating.value && (recordId.value !== '' || valueLines.value !== ''))
type Values = Record<string, { kind: { case: 'stringValue'; value: string } }>
function parseValues(): Values | string {
  const out: Values = {}
  for (const line of valueLines.value.split('\n')) {
    if (!line.trim()) continue
    const at = line.indexOf('=')
    if (at < 1) return `"${line}" is not field=value.`
    out[line.slice(0, at).trim()] = { kind: { case: 'stringValue', value: line.slice(at + 1).trim() } }
  }
  return out
}
async function simulate() {
  const values = parseValues()
  if (typeof values === 'string') {
    toast.error(values)
    return
  }
  busy.value = true
  try {
    simulation.value = await ruleClient.simulateRule({ definition: toDefinition(), recordId: recordId.value.trim(), values })
  } catch (err) {
    simulation.value = null
    toast.error(describe(err, 'simulate the Rule'))
  } finally {
    busy.value = false
  }
}

function microseconds(value: bigint): string {
  const n = Number(value)
  return n >= 1000 ? `${(n / 1000).toFixed(1)} ms` : `${n} µs`
}
onMounted(refresh)
</script>

<template>
  <section class="page-content files-page">
    <div class="page-title-row">
      <div><h1>Rules <HelpHint term="rule" /></h1><p class="lead">Starlark programs that check, fill in and guard records as they are saved.</p></div>
      <div v-if="canWrite && !unavailable" class="toolbar-actions"><ActionButton kind="primary" label="New Rule" icon="plus" @press="startNew" /></div>
    </div>
    <div v-if="unavailable" class="empty-builder"><div class="card-icon">⌘</div><h2>Rules are unavailable</h2><p>{{ unavailable }}</p></div>
    <div v-else>
      <DataTable
        :columns="ruleColumns"
        :data="filteredRules"
        table-id="rules-list"
        :username="username"
        :get-row-id="(rule) => rule.definition?.key ?? ''"
        :row-actions="() => canWrite ? [{ id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2', busy: busy }] : []"
        v-model:search="ruleSearch"
        search-placeholder="Find rules…"
        :filters="ruleFilters"
        :loading="!loaded"
        :filtered="ruleFilteredActive"
        clickable
        empty-title="No Rules yet"
        empty-body="Create one to get started."
        empty-art="empty-rules"
        filtered-art="empty-sieve"
        @row-click="choose"
        @row-action="onRuleRowAction"
        @update:filters="onRuleFiltersChanged"
      >
        <template v-if="canWrite" #empty-action>
          <ActionButton kind="primary" label="Create your first Rule" icon="plus" @press="startNew" />
        </template>
      </DataTable>
      <div class="record-editor files-editor-card mt-4">
        <p v-if="!isOpen" class="files-empty">{{ loaded && rules.length === 0 ? 'Rules calculate from the record itself; they cannot reach files, the network or the time of day.' : 'Select a Rule, or create one.' }}</p>
        <template v-else>
          <div class="record-editor-head">
            <h2 class="files-title">{{ creating ? 'New Rule' : selected }}</h2>
            <Badge v-if="!creating" variant="secondary">v{{ version }}</Badge>
            <Badge v-if="dirty" variant="outline">Unsaved</Badge>
            <div class="toolbar-actions">
              <ActionButton v-if="canWrite" kind="primary" label="Save" icon="save" :busy="busy" :disabled-reason="!dirty ? 'No changes to save.' : problem" @press="save" />
              <ActionButton v-if="canWrite" kind="secondary" label="Simulate" icon="play" :busy="busy" :disabled-reason="problem" @press="simulation = null; simulating = true" />
              <ActionButton v-if="canWrite && !creating" kind="destructive" label="Delete" icon="trash-2" :busy="busy" @press="askDeleteSelected" />
            </div>
          </div>
          <Tabs :model-value="detail" @update:model-value="onDetail">
            <TabsList>
              <TabsTrigger value="editor">Editor</TabsTrigger>
              <TabsTrigger value="runs" :disabled="creating">Runs</TabsTrigger>
              <TabsTrigger value="versions" :disabled="creating">Versions</TabsTrigger>
            </TabsList>
            <TabsContent value="editor">
              <div class="settings-grid">
                <div class="settings-field"><Label for="rule-key">Key</Label><Input id="rule-key" v-model="form.key" :disabled="!creating" autocomplete="off" spellcheck="false" /></div>
                <div class="settings-field"><Label for="rule-label">Label</Label><Input id="rule-label" v-model="form.label" :disabled="!canWrite" autocomplete="off" /></div>
                <div class="settings-field">
                  <Label for="rule-type">Type</Label>
                  <Select v-model="form.ruleType" :disabled="!canWrite">
                    <SelectTrigger id="rule-type"><SelectValue placeholder="Type" /></SelectTrigger>
                    <SelectContent><SelectItem v-for="type in ruleTypes" :key="type" :value="type">{{ type }}</SelectItem></SelectContent>
                  </Select>
                  <small class="files-sub">{{ isRuleType(form.ruleType) ? ruleTypeHelp[form.ruleType] : '' }}</small>
                </div>
                <div class="settings-field">
                  <Label for="rule-record-type">Record Type</Label>
                  <Select v-model="form.recordTypeKey" :disabled="!canWrite || form.ruleType === 'transform'">
                    <SelectTrigger id="rule-record-type"><SelectValue :placeholder="form.ruleType === 'transform' ? 'Not used' : 'Document Record Type'" /></SelectTrigger>
                    <SelectContent><SelectItem v-for="item in documentTypes" :key="item.name" :value="item.name">{{ item.label }}</SelectItem></SelectContent>
                  </Select>
                  <small v-if="documentTypes.length === 0 && form.ruleType !== 'transform'" class="files-sub">No Document Record Types yet. Create one under Record Types first.</small>
                </div>
                <div class="settings-field">
                  <Label for="rule-event">Event</Label>
                  <Select v-model="form.event" :disabled="!canWrite || events.length === 0">
                    <SelectTrigger id="rule-event"><SelectValue :placeholder="events.length === 0 ? 'Runs when called' : 'Event'" /></SelectTrigger>
                    <SelectContent><SelectItem v-for="event in events" :key="event" :value="event">{{ event }}</SelectItem></SelectContent>
                  </Select>
                </div>
                <div class="settings-field"><Label for="rule-priority">Priority</Label><Input id="rule-priority" v-model.number="form.priority" type="number" min="0" max="1000" :disabled="!canWrite" /></div>
                <div class="settings-field"><Label for="rule-enabled">Enabled</Label><Switch id="rule-enabled" v-model="form.enabled" :disabled="!canWrite" /></div>
              </div>
              <div v-if="builderOffered" class="mb-3 flex items-center gap-2" role="group" aria-label="Authoring mode">
                <button
                  type="button"
                  class="h-8 rounded-3xl border px-3 text-sm"
                  :class="form.mode === 'builder' ? 'border-transparent bg-primary text-primary-foreground' : 'border-border text-muted-foreground'"
                  :aria-pressed="form.mode === 'builder'"
                  :disabled="!canWrite"
                  @click="askSwitch('builder')"
                >
                  Builder
                </button>
                <button
                  type="button"
                  class="h-8 rounded-3xl border px-3 text-sm"
                  :class="form.mode === 'starlark' ? 'border-transparent bg-primary text-primary-foreground' : 'border-border text-muted-foreground'"
                  :aria-pressed="form.mode === 'starlark'"
                  :disabled="!canWrite"
                  @click="askSwitch('starlark')"
                >
                  Starlark
                </button>
              </div>
              <template v-if="form.mode === 'builder' && builderOffered">
                <p v-if="!form.recordTypeKey" class="files-hint">Pick the Record Type the Rule belongs to, then build its condition.</p>
                <ConditionBuilder v-else :model-value="form.tree" :fields="builderFields" @update:model-value="form = { ...form, tree: $event }" />
                <div v-if="form.ruleType === 'validation' || form.ruleType === 'guard'" class="settings-field mt-3">
                  <Label for="rule-message">{{ form.ruleType === 'validation' ? 'Message shown when the check fails' : 'Reason shown when blocked' }}</Label>
                  <Input id="rule-message" v-model="form.message" :disabled="!canWrite" autocomplete="off" placeholder="Say what to fix, in plain words" />
                </div>
                <div v-if="form.ruleType === 'derivation'" class="mt-3 space-y-2">
                  <p class="text-sm font-medium">Set these fields when the condition holds</p>
                  <div v-for="(set, index) in form.sets" :key="index" class="flex flex-wrap items-center gap-2">
                    <Select :model-value="set.field" :disabled="!canWrite" @update:model-value="updateSet(index, { field: String($event) })">
                      <SelectTrigger class="w-48" :aria-label="`Field for set ${index + 1}`"><SelectValue placeholder="Field" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem v-for="f in builderFields" :key="f.name" :value="f.name">{{ f.label }}</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input :model-value="set.valueExpr" :disabled="!canWrite" :aria-label="`Value for set ${index + 1}`" placeholder="Value or expression" class="min-w-40 flex-1" @update:model-value="updateSet(index, { valueExpr: String($event) })" />
                    <ActionButton kind="quiet" density="row" label="Remove" icon="x" @press="removeSet(index)" />
                  </div>
                  <ActionButton kind="secondary" density="row" label="Set a field" icon="plus" :disabled="!canWrite" @press="addSet" />
                </div>
              </template>
              <Textarea v-else v-model="form.source" class="files-editor" spellcheck="false" :readonly="!canWrite" aria-label="Starlark source" @keydown.meta.s.prevent="save" @keydown.ctrl.s.prevent="save" />
              <p v-if="problem && dirty" class="files-hint files-problem">{{ problem }}</p>
              <p v-else-if="form.mode !== 'builder' || !builderOffered" class="files-hint">The server compiles the source on save and reports the line of a mistake.</p>
              <p v-else class="files-hint">The server compiles the condition on save and runs it without Starlark.</p>
            </TabsContent>
            <TabsContent value="runs">
              <DataTable
                :key="selected"
                :columns="runColumns"
                :data="filteredRuns"
                table-id="rule-runs"
                :username="username"
                :get-row-id="(run) => run.id"
                v-model:search="runSearch"
                search-placeholder="Find runs…"
                :filters="runFilters"
                :filtered="runFilteredActive"
                clickable
                empty-title="No runs yet"
                empty-body="This Rule has not run yet."
                empty-art="empty-runs"
                filtered-art="empty-sieve"
                @row-click="openRun"
                @update:filters="onRunFiltersChanged"
              />
            </TabsContent>
            <TabsContent value="versions">
              <Table class="files-table">
                <TableHeader><TableRow><TableHead>Version</TableHead><TableHead>When</TableHead><TableHead>By</TableHead><TableHead>Hash</TableHead><TableHead /></TableRow></TableHeader>
                <TableBody>
                  <TableRow v-for="item in versions" :key="String(item.version)">
                    <TableCell>v{{ item.version }}</TableCell>
                    <TableCell>{{ formatWhen(item.createdAt) }}</TableCell>
                    <TableCell>{{ item.actor }}</TableCell>
                    <TableCell><code>{{ item.sourceHash.slice(0, 10) }}</code></TableCell>
                    <TableCell><ActionButton v-if="canWrite" kind="restore" density="row" label="Load into editor" icon="rotate-ccw" :busy="busy" @press="loadVersion(item)" /></TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </TabsContent>
          </Tabs>
        </template>
      </div>
    </div>
    <ResponsiveDialog
      v-model:open="simulating"
      :title="'Simulate ' + (form.key || 'Rule')"
      description="A dry run of the source in the editor. Nothing is saved; the run is logged as a dry run. Reads follow your own permissions."
      :dirty="simulateDirty"
    >
        <form id="rule-simulate" class="sign-in-form" @submit.prevent="simulate">
          <div class="settings-field"><Label for="sim-record">Stored Record ID (optional)</Label><Input id="sim-record" v-model="recordId" autocomplete="off" spellcheck="false" /></div>
          <div class="settings-field"><Label for="sim-values">Values, one field=value per line</Label><Textarea id="sim-values" v-model="valueLines" spellcheck="false" placeholder="qty=2&#10;rate=1.50" /></div>
        </form>
        <div v-if="simulation" class="settings-field" role="status">
          <p><Badge :variant="simulation.outcome === 'ok' ? 'secondary' : 'outline'">{{ simulation.outcome }}</Badge> <small class="files-sub">{{ simulation.code }} {{ simulation.message }}</small></p>
          <Table v-if="simulation.changes.length" class="files-table">
            <TableHeader><TableRow><TableHead>Field</TableHead><TableHead>Before</TableHead><TableHead>After</TableHead></TableRow></TableHeader>
            <TableBody><TableRow v-for="change in simulation.changes" :key="change.field"><TableCell><code>{{ change.field }}</code></TableCell><TableCell>{{ change.oldValue ?? '(empty)' }}</TableCell><TableCell>{{ change.newValue ?? '(empty)' }}</TableCell></TableRow></TableBody>
          </Table>
          <p v-for="(w, i) in simulation.warnings" :key="`w${i}`" class="files-hint">Warning {{ w.code }}: {{ w.text }}</p>
          <p v-for="(l, i) in simulation.logs" :key="`l${i}`" class="files-hint">log {{ l.code }}: {{ l.text }}</p>
          <p class="files-hint">{{ simulation.steps }} steps, {{ simulation.reads }} reads, {{ microseconds(simulation.durationMicros) }}, input <code>{{ simulation.inputHash.slice(0, 10) }}</code></p>
        </div>
        <template #footer>
          <ActionButton kind="secondary" label="Close" icon="x" @press="simulating = false" />
          <ActionButton type="submit" form="rule-simulate" kind="primary" label="Run" icon="play" :busy="busy" />
        </template>
    </ResponsiveDialog>
    <ConfirmDialog
      v-model:open="deleting"
      :title="`Delete ${deletingRule?.key ?? selected}?`"
      body="The Rule stops running. Its versions and Rule Runs stay in the log."
      confirm-label="Delete Rule"
      cancel-label="Keep it"
      :busy="busy"
      @confirm="remove"
      @update:open="(value: boolean) => { if (!value) deletingRule = null }"
    />
    <ConfirmDialog
      :open="pending !== null"
      title="Discard your edits?"
      body="The Rule has changes that are not saved."
      confirm-label="Discard edits"
      cancel-label="Keep editing"
      confirm-icon="x"
      @confirm="proceed"
      @update:open="(value: boolean) => { if (!value) pending = null }"
    />
    <ConfirmDialog
      :open="switchTo !== null"
      :title="switchTo === 'starlark' ? 'Write Starlark instead?' : 'Start over in the builder?'"
      :body="switchTo === 'starlark'
        ? 'The condition becomes a commented scaffold at the top of the source. Saving clears the builder fields.'
        : 'The Starlark source is discarded and the builder starts blank. Saving clears the source.'"
      :confirm-label="switchTo === 'starlark' ? 'Switch to Starlark' : 'Start over'"
      cancel-label="Keep editing"
      @confirm="confirmSwitch"
      @update:open="(value: boolean) => { if (!value) switchTo = null }"
    />

    <DetailSheet
      v-if="selectedRun"
      :open="true"
      title="Rule run"
      @update:open="(value: boolean) => { if (!value) selectedRun = null }"
    >
      <template #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ selectedRun.id }}</p>
      </template>
      <template #status>
        <Badge :variant="selectedRun.outcome === 'ok' ? 'secondary' : 'outline'">{{ selectedRun.outcome }}</Badge>
        <Badge v-if="selectedRun.dryRun" variant="outline">Dry run</Badge>
      </template>
      <div class="sheet-section">
        <p class="sheet-label">Outcome</p>
        <p class="sheet-lead">{{ selectedRun.message || selectedRun.code || selectedRun.outcome }}</p>
        <p class="muted-label">{{ formatWhen(selectedRun.createdAt) }} · v{{ selectedRun.ruleVersion }} · {{ selectedRun.event || 'called' }} · {{ selectedRun.actor }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Messages</p>
        <ul v-if="selectedRun.warnings.length + selectedRun.logs.length > 0" class="sheet-events">
          <li v-for="(entry, index) in [...selectedRun.warnings, ...selectedRun.logs]" :key="index"><span>{{ entry.text }}</span><span class="muted-label">{{ entry.code }}</span></li>
        </ul>
        <p v-else class="files-empty">The rule reported nothing.</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Input</p>
        <p class="text-caption font-mono">{{ selectedRun.inputHash }}</p>
        <p v-if="selectedRun.recordId" class="muted-label">For {{ selectedRun.recordTypeKey }} {{ selectedRun.recordId }}</p>
        <p class="muted-label">{{ selectedRun.steps }} steps · {{ selectedRun.reads }} reads · {{ selectedRun.writes }} writes · {{ microseconds(selectedRun.durationMicros) }}</p>
      </div>
    </DetailSheet>
  </section>
</template>
