<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ruleClient } from '../api'
import type { Rule, RuleRun, RuleVersion, SimulateRuleResponse } from '../gen/athena/v1/rules_pb'
import { isRuleType, ruleEvents, ruleProblem, ruleTypeHelp, ruleTypes, starterSource, type RuleType } from '../ruleDefinitions'
import type { RecordType } from '../types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'

// The Rules page edits Starlark Rules through RuleService. The server compiles
// every source on save and returns the line of a mistake; ruleDefinitions.ts
// only repeats the definition rules so a slip shows early. Starlark Rules are
// not production-ready (ADR-0016, Proposed).
const props = defineProps<{ canWrite: boolean; recordTypes: RecordType[] }>()

type Form = { key: string; label: string; ruleType: string; recordTypeKey: string; event: string; priority: number; enabled: boolean; source: string }
const blank = (): Form => ({ key: '', label: '', ruleType: 'validation', recordTypeKey: '', event: 'validate', priority: 100, enabled: true, source: starterSource.validation })

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
const problem = computed(() => (selected.value || creating.value ? ruleProblem(form.value) : null))
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
  return { key: d?.key ?? '', label: d?.label ?? '', ruleType: d?.ruleType ?? 'validation', recordTypeKey: d?.recordTypeKey ?? '', event: d?.event ?? '', priority: d?.priority ?? 100, enabled: d?.enabled ?? true, source: d?.source ?? '' }
}

async function refresh() {
  try {
    rules.value = (await ruleClient.listRules({})).rules
    unavailable.value = ''
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.Unavailable) unavailable.value = err.rawMessage
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
  })
}
function proceed() {
  const action = pending.value
  pending.value = null
  action?.()
}

// Changing the type resets what depends on it.
watch(() => form.value.ruleType, (type) => {
  if (!isRuleType(type)) return
  const allowed = ruleEvents[type]
  if (!allowed.includes(form.value.event)) form.value.event = allowed[0] ?? ''
  if (type === 'transform') form.value.recordTypeKey = ''
  if (creating.value && form.value.source === starterSource[previousType.value]) form.value.source = starterSource[type]
  previousType.value = type
})

async function save() {
  if (!props.canWrite || !dirty.value || problem.value) return
  busy.value = true
  try {
    const res = await ruleClient.saveRule({ definition: form.value, expectedVersion: BigInt(version.value) })
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
  if (!selected.value) return
  try {
    runs.value = (await ruleClient.listRuleRuns({ ruleKey: selected.value, pageSize: 50 })).runs
  } catch (err) {
    toast.error(describe(err, 'load the runs'))
  }
}
async function loadVersions() {
  if (!selected.value) return
  try {
    versions.value = (await ruleClient.listRuleVersions({ key: selected.value })).versions
  } catch (err) {
    toast.error(describe(err, 'load the versions'))
  }
}
function onDetail(value: string | number) {
  detail.value = value === 'runs' ? 'runs' : value === 'versions' ? 'versions' : 'editor'
  if (detail.value === 'runs') void loadRuns()
  if (detail.value === 'versions') void loadVersions()
}
function loadVersion(item: RuleVersion) {
  const d = item.definition
  if (!d) return
  form.value = { ...form.value, label: d.label, ruleType: d.ruleType, recordTypeKey: d.recordTypeKey, event: d.event, priority: d.priority, enabled: d.enabled, source: d.source }
  detail.value = 'editor'
  toast.success(`Version ${item.version} is in the editor. Save to make it the current version.`)
}

const deleting = ref(false)
async function remove() {
  deleting.value = false
  busy.value = true
  try {
    await ruleClient.deleteRule({ key: selected.value, expectedVersion: BigInt(version.value) })
    toast.success(`Deleted ${selected.value}. Its versions and runs are kept.`)
    selected.value = ''
    form.value = blank()
    saved.value = blank()
    await refresh()
  } catch (err) {
    toast.error(describe(err, 'delete the Rule'))
  } finally {
    busy.value = false
  }
}

// Simulate: a rolled-back dry run of the edited source.
const simulating = ref(false)
const simulation = ref<SimulateRuleResponse | null>(null)
const recordId = ref('')
const valueLines = ref('')
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
    simulation.value = await ruleClient.simulateRule({ definition: form.value, recordId: recordId.value.trim(), values })
  } catch (err) {
    simulation.value = null
    toast.error(describe(err, 'simulate the Rule'))
  } finally {
    busy.value = false
  }
}

function when(iso: string): string {
  return iso ? new Date(iso).toLocaleString() : ''
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
      <div><h1>Rules</h1><p class="lead">Starlark programs that validate, derive and guard documents as they are saved. Go checks permissions and limits every run.</p></div>
      <div v-if="canWrite && !unavailable" class="toolbar-actions"><Button @click="startNew">＋ New Rule</Button></div>
    </div>
    <div v-if="unavailable" class="empty-builder"><div class="card-icon">⌘</div><h2>Rules are unavailable</h2><p>{{ unavailable }}</p></div>
    <div v-else class="files-layout">
      <nav class="files-tree" aria-label="Rules">
        <p v-if="loaded && rules.length === 0" class="files-empty">No Rules yet. {{ canWrite ? 'Create one.' : '' }}</p>
        <button v-for="rule in rules" :key="rule.definition?.key" class="files-node" :class="{ chosen: rule.definition?.key === selected && !creating }" :title="rule.definition?.label" @click="choose(rule)">
          <span class="files-name">{{ rule.definition?.key }}</span>
          <span class="files-flag">{{ rule.definition?.enabled ? rule.definition?.ruleType : 'off' }}</span>
        </button>
      </nav>
      <div class="record-editor files-editor-card">
        <p v-if="!isOpen" class="files-empty">Select a Rule, or create one. A Rule cannot reach SQL, files, the network, the clock or floating-point numbers.</p>
        <template v-else>
          <div class="record-editor-head">
            <h2 class="files-title">{{ creating ? 'New Rule' : selected }}</h2>
            <Badge v-if="!creating" variant="secondary">v{{ version }}</Badge>
            <Badge v-if="dirty" variant="outline">Unsaved</Badge>
            <div class="toolbar-actions">
              <Button v-if="canWrite" :disabled="!dirty || busy || problem !== null" @click="save">Save</Button>
              <Button v-if="canWrite" variant="outline" :disabled="busy || problem !== null" @click="simulation = null; simulating = true">Simulate</Button>
              <Button v-if="canWrite && !creating" variant="ghost" :disabled="busy" @click="deleting = true">Delete</Button>
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
              <Textarea v-model="form.source" class="files-editor" spellcheck="false" :readonly="!canWrite" aria-label="Starlark source" @keydown.meta.s.prevent="save" @keydown.ctrl.s.prevent="save" />
              <p v-if="problem && dirty" class="files-hint files-problem">{{ problem }}</p>
              <p v-else class="files-hint">The server compiles the source on save and reports the line of a mistake.</p>
            </TabsContent>
            <TabsContent value="runs">
              <Table class="files-table">
                <TableHeader><TableRow><TableHead>When</TableHead><TableHead>Version</TableHead><TableHead>Event</TableHead><TableHead>Outcome</TableHead><TableHead>Steps</TableHead><TableHead>Duration</TableHead><TableHead>By</TableHead><TableHead>Input</TableHead></TableRow></TableHeader>
                <TableBody>
                  <TableRow v-for="run in runs" :key="run.id">
                    <TableCell>{{ when(run.createdAt) }}</TableCell>
                    <TableCell>v{{ run.ruleVersion }}</TableCell>
                    <TableCell>{{ run.event || 'called' }}<small v-if="run.dryRun" class="files-sub"> dry run</small></TableCell>
                    <TableCell><Badge :variant="run.outcome === 'ok' ? 'secondary' : 'outline'">{{ run.outcome }}</Badge><small v-if="run.code" class="files-sub"> {{ run.code }}</small></TableCell>
                    <TableCell>{{ run.steps }}</TableCell>
                    <TableCell>{{ microseconds(run.durationMicros) }}</TableCell>
                    <TableCell>{{ run.actor }}</TableCell>
                    <TableCell><code>{{ run.inputHash.slice(0, 10) }}</code></TableCell>
                  </TableRow>
                </TableBody>
              </Table>
              <p v-if="runs.length === 0" class="files-hint">This Rule has not run yet.</p>
            </TabsContent>
            <TabsContent value="versions">
              <Table class="files-table">
                <TableHeader><TableRow><TableHead>Version</TableHead><TableHead>When</TableHead><TableHead>By</TableHead><TableHead>Hash</TableHead><TableHead /></TableRow></TableHeader>
                <TableBody>
                  <TableRow v-for="item in versions" :key="String(item.version)">
                    <TableCell>v{{ item.version }}</TableCell>
                    <TableCell>{{ when(item.createdAt) }}</TableCell>
                    <TableCell>{{ item.actor }}</TableCell>
                    <TableCell><code>{{ item.sourceHash.slice(0, 10) }}</code></TableCell>
                    <TableCell><Button v-if="canWrite" variant="ghost" size="sm" @click="loadVersion(item)">Load into editor</Button></TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </TabsContent>
          </Tabs>
        </template>
      </div>
    </div>
    <Dialog :open="simulating" @update:open="(value: boolean) => { simulating = value }">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Simulate {{ form.key || 'Rule' }}</DialogTitle>
          <DialogDescription>A dry run of the source in the editor. Nothing is saved; the run is logged as a dry run. Reads follow your own permissions.</DialogDescription>
        </DialogHeader>
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
        <DialogFooter>
          <Button variant="outline" @click="simulating = false">Close</Button>
          <Button type="submit" form="rule-simulate" :disabled="busy">Run</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    <AlertDialog :open="deleting" @update:open="(value: boolean) => { deleting = value }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Delete {{ selected }}?</AlertDialogTitle><AlertDialogDescription>The Rule stops running. Its versions and Rule Runs stay in the log.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep it</AlertDialogCancel><AlertDialogAction variant="destructive" @click="remove">Delete Rule</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
    <AlertDialog :open="pending !== null" @update:open="(value: boolean) => { if (!value) pending = null }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Discard your edits?</AlertDialogTitle><AlertDialogDescription>The Rule has changes that are not saved.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction variant="destructive" @click="proceed">Discard edits</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </section>
</template>
