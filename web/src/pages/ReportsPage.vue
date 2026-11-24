// Reports: saved lists and totals from Records. Lists run live through
// ListService; Export PDF downloads the rendered bytes; Email/Message queue
// through a confirm sheet with audience counts. Totals are still on the way.
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Code, ConnectError } from '@connectrpc/connect'
import ActionButton from '@/components/ActionButton.vue'
import ConditionBuilder from '@/components/conditions/ConditionBuilder.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import EmptyState from '@/components/EmptyState.vue'
import PageHeader from '@/components/PageHeader.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import FormSection from '@/components/FormSection.vue'
import TextField from '@/components/fields/TextField.vue'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { listClient } from '@/api'
import { builderFieldsFor, type BuilderField, type ConditionTree } from '@/conditions'
import { labelWithState, resolveRecords } from '@/lookup'
import { recordFieldText } from '@/components/records/recordColumns'
import type { Record as RecordMessage } from '@/gen/athena/v1/records_pb'
import type { SavedList } from '@/gen/athena/v1/lists_pb'
import type { Field, RecordType } from '@/types'

const props = defineProps<{
  canManage: boolean
  canSend: boolean
  recordTypes: RecordType[]
}>()

const savedTypes = computed(() => props.recordTypes.filter((t) => t.revision > 0))
const selectedType = ref('')
watch(savedTypes, (types) => {
  if (!types.some((t) => t.name === selectedType.value)) selectedType.value = types[0]?.name ?? ''
}, { immediate: true })
const currentType = computed(() => props.recordTypes.find((t) => t.name === selectedType.value) ?? null)
const builderFields = computed<BuilderField[]>(() => builderFieldsFor(currentType.value?.fields ?? []))

function failureMessage(err: unknown, action: string): string {
  if (err instanceof ConnectError) {
    if (err.code === Code.PermissionDenied) return 'You do not have permission to do this.'
    if (err.rawMessage) return err.rawMessage
  }
  return `Could not ${action}.`
}

// Lists of the selected type.
const lists = ref<SavedList[]>([])
const listsLoading = ref(false)
const listsFailed = ref('')

async function loadLists() {
  if (!selectedType.value) {
    lists.value = []
    return
  }
  listsLoading.value = true
  listsFailed.value = ''
  try {
    const res = await listClient.listLists({ recordTypeKey: selectedType.value, pageSize: 100 })
    lists.value = res.lists
  } catch (err) {
    listsFailed.value = failureMessage(err, 'load the lists')
  } finally {
    listsLoading.value = false
  }
}

watch(selectedType, () => {
  openListId.value = ''
  void loadLists()
})

// The open list and its run.
const openListId = ref('')
const openList = computed(() => lists.value.find((l) => l.id === openListId.value) ?? null)
const runRows = ref<RecordMessage[]>([])
const runToken = ref('')
const runLoading = ref(false)
const runFailed = ref('')

async function openListById(id: string) {
  openListId.value = id
  runRows.value = []
  runToken.value = ''
  runFailed.value = ''
  await runMore()
}

async function runMore() {
  if (!openListId.value || runLoading.value) return
  runLoading.value = true
  runFailed.value = ''
  try {
    const res = await listClient.runList({ id: openListId.value, pageSize: 50, pageToken: runToken.value })
    runRows.value = [...runRows.value, ...res.records]
    runToken.value = res.nextPageToken
  } catch (err) {
    runFailed.value = failureMessage(err, 'run the list')
  } finally {
    runLoading.value = false
  }
}

const runColumns = computed(() => {
  const fields = currentType.value?.fields ?? []
  const wanted = openList.value?.columns ?? []
  if (wanted.length === 0) return fields
  const byName = new Map(fields.map((f) => [f.name, f]))
  return wanted.map((name) => byName.get(name)).filter((f): f is Field => !!f)
})

function cellText(field: Field, record: RecordMessage): string {
  return recordFieldText(field, record, linkLabels.value)
}

// Link cells show the linked Record's label, never its ID. Labels load once
// per run and fill in as they arrive.
const linkLabels = ref<Record<string, string>>({})
watch(runRows, async (items) => {
  if (!currentType.value) return
  for (const field of currentType.value.fields) {
    if (field.type !== 'Link' || !field.target_record_type) continue
    const ids = items.flatMap((record) => { const kind = record.values[field.name]?.kind; return kind?.case === 'stringValue' ? [kind.value] : [] })
    if (ids.length === 0) continue
    try {
      const found = await resolveRecords(field.target_record_type, ids)
      linkLabels.value = { ...linkLabels.value, ...Object.fromEntries([...found].map(([id, item]) => [id, labelWithState(item)])) }
    } catch {
      // The cell stays empty; the run table shows lookup errors on retry.
    }
  }
})

// PDF export downloads the rendered bytes.
const exporting = ref(false)

async function exportPdf() {
  if (!openListId.value || exporting.value) return
  exporting.value = true
  try {
    const res = await listClient.exportListPdf({ id: openListId.value, maxRows: 200 })
    const url = URL.createObjectURL(new Blob([new Uint8Array(res.pdf)], { type: 'application/pdf' }))
    const link = document.createElement('a')
    link.href = url
    link.download = res.filename || 'list.pdf'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    toast.success('The PDF is downloading.')
  } catch (err) {
    toast.error(failureMessage(err, 'export the list'))
  } finally {
    exporting.value = false
  }
}

// Manage dialog: name, condition, columns.
const manageOpen = ref(false)
const managing = ref<{ id: string; revision: bigint }>({ id: '', revision: 0n })
const manageName = ref('')
const manageTree = ref<ConditionTree | null>(null)
const manageColumns = ref<string[]>([])
const manageBusy = ref(false)
const manageFailed = ref('')

function openNew() {
  managing.value = { id: '', revision: 0n }
  manageName.value = ''
  manageTree.value = null
  manageColumns.value = []
  manageFailed.value = ''
  manageOpen.value = true
}

function openEdit() {
  const list = openList.value
  if (!list) return
  let tree: ConditionTree | null = null
  try {
    tree = JSON.parse(list.conditionTree) as ConditionTree
  } catch {
    tree = null
  }
  managing.value = { id: list.id, revision: list.revision }
  manageName.value = list.name
  manageTree.value = tree
  manageColumns.value = [...list.columns]
  manageFailed.value = ''
  manageOpen.value = true
}

function toggleColumn(name: string, on: boolean) {
  manageColumns.value = on ? [...manageColumns.value, name] : manageColumns.value.filter((c) => c !== name)
}

async function saveManaged() {
  if (manageBusy.value) return
  if (!manageName.value.trim()) {
    manageFailed.value = 'Give the list a name.'
    return
  }
  manageBusy.value = true
  manageFailed.value = ''
  try {
    await listClient.saveList({
      id: managing.value.id,
      name: manageName.value.trim(),
      recordTypeKey: selectedType.value,
      conditionTree: JSON.stringify(manageTree.value ?? { root: { all: true, conditions: [] } }),
      columns: manageColumns.value,
      expectedRevision: managing.value.revision,
    })
    manageOpen.value = false
    toast.success('The list is saved.')
    await loadLists()
  } catch (err) {
    manageFailed.value = failureMessage(err, 'save the list')
  } finally {
    manageBusy.value = false
  }
}

const deleting = ref(false)

async function deleteOpen() {
  const list = openList.value
  if (!list || deleting.value) return
  deleting.value = true
  try {
    await listClient.deleteList({ id: list.id, expectedRevision: list.revision })
    openListId.value = ''
    toast.success('The list is deleted.')
    await loadLists()
  } catch (err) {
    toast.error(failureMessage(err, 'delete the list'))
  } finally {
    deleting.value = false
  }
}

// Queue dialog: channel, optional audience, preview counts, confirm.
const queueOpen = ref(false)
const queueChannel = ref<'email' | 'message'>('email')
const queueAudience = ref<ConditionTree | null>(null)
const queueBusy = ref(false)
const queueFailed = ref('')
const queuePreview = ref<{ queued: bigint; excluded: bigint; total: bigint } | null>(null)

function openQueue() {
  queueOpen.value = true
  queueChannel.value = 'email'
  queueAudience.value = null
  queueFailed.value = ''
  queuePreview.value = null
}

async function previewQueue() {
  if (!openListId.value || queueBusy.value) return
  queueBusy.value = true
  queueFailed.value = ''
  try {
    const res = await listClient.queueMessages({
      id: openListId.value,
      channel: queueChannel.value,
      audienceTree: queueAudience.value ? JSON.stringify(queueAudience.value) : '',
      maxRecipients: 200,
      dryRun: true,
    })
    queuePreview.value = { queued: res.queued, excluded: res.excluded, total: res.total }
  } catch (err) {
    queueFailed.value = failureMessage(err, 'preview the send')
  } finally {
    queueBusy.value = false
  }
}

async function confirmQueue() {
  if (!openListId.value || queueBusy.value) return
  queueBusy.value = true
  queueFailed.value = ''
  try {
    const res = await listClient.queueMessages({
      id: openListId.value,
      channel: queueChannel.value,
      audienceTree: queueAudience.value ? JSON.stringify(queueAudience.value) : '',
      maxRecipients: 200,
    })
    queueOpen.value = false
    toast.success(`Queued ${res.queued} of ${res.total} matching rows. Nothing sends in beta.`)
  } catch (err) {
    queueFailed.value = failureMessage(err, 'queue the send')
  } finally {
    queueBusy.value = false
  }
}

const confirmDeleteOpen = ref(false)

async function confirmDelete() {
  confirmDeleteOpen.value = false
  await deleteOpen()
}
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Reports" description="Lists and totals from your records." />
    <FormSection title="Saved lists" description="A saved list reruns its filter any time.">
      <div class="mb-4 flex flex-wrap items-center gap-2">
        <NativeSelect v-model="selectedType" aria-label="Record Type">
          <NativeSelectOption v-for="t in savedTypes" :key="t.name" :value="t.name">{{ t.label }}</NativeSelectOption>
        </NativeSelect>
        <ActionButton v-if="canManage" kind="primary" density="row" label="New list" icon="plus" :disabled="!selectedType" @press="openNew" />
      </div>
      <p v-if="listsLoading" class="text-muted-foreground text-sm">Loading the lists…</p>
      <p v-else-if="listsFailed" class="text-destructive text-sm" role="alert">{{ listsFailed }}</p>
      <EmptyState
        v-else-if="lists.length === 0"
        title="No lists yet"
        :body="canManage ? 'Save a list to rerun a filter any time.' : 'A builder can save the first list.'"
      />
      <ul v-else class="divide-border divide-y rounded-lg border">
        <li v-for="list in lists" :key="list.id">
          <button type="button" class="hover:bg-muted/50 flex w-full items-center justify-between gap-2 px-3 py-2 text-left" :aria-current="list.id === openListId || undefined" @click="openListById(list.id)">
            <span class="text-sm font-medium">{{ list.name }}</span>
            <span class="text-muted-foreground text-xs">Open</span>
          </button>
        </li>
      </ul>
      <div v-if="openList" class="mt-6 space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-base font-medium">{{ openList.name }}</h3>
          <span class="text-muted-foreground text-sm">{{ runRows.length }} shown</span>
          <span class="flex-1" />
          <ActionButton kind="secondary" density="row" label="Export PDF" icon="download" :loading="exporting" @press="exportPdf" />
          <ActionButton v-if="canSend" kind="secondary" density="row" label="Email them" icon="send" @press="openQueue" />
          <ActionButton v-if="canSend" kind="secondary" density="row" label="Message them" icon="send" @press="openQueue" />
          <ActionButton v-if="canManage" kind="quiet" density="row" label="Edit" icon="pencil" @press="openEdit" />
          <ActionButton v-if="canManage" kind="quiet" density="row" label="Delete" icon="trash-2" :loading="deleting" @press="confirmDeleteOpen = true" />
        </div>
        <p v-if="runLoading && runRows.length === 0" class="text-muted-foreground text-sm">Running the list…</p>
        <p v-else-if="runFailed" class="text-destructive text-sm" role="alert">{{ runFailed }}</p>
        <EmptyState v-else-if="runRows.length === 0" title="No matching records" body="Loosen the filter or add records." />
        <Table v-else>
          <TableHeader>
            <TableRow>
              <TableHead v-for="col in runColumns" :key="col.name">{{ col.label }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in runRows" :key="row.id">
              <TableCell v-for="col in runColumns" :key="col.name">{{ cellText(col, row) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <ActionButton v-if="runToken" kind="secondary" density="row" label="Show more" :loading="runLoading" @press="runMore" />
      </div>
    </FormSection>

    <FormSection title="Totals" description="Sums and counts across a list.">
      <EmptyState art="empty-reports" title="Totals are on the way" body="Totals arrive in a later batch; lists work today." />
    </FormSection>

    <ResponsiveDialog v-model:open="manageOpen" title="Save list" :wide="true" :dismissible="!manageBusy">
      <TextField id="list-name" v-model="manageName" label="Name" :disabled="manageBusy" autocomplete="off" placeholder="RSVP yes, missing details" />
      <p class="mt-3 text-sm font-medium">Conditions</p>
      <ConditionBuilder v-model="manageTree" :fields="builderFields" />
      <p class="mt-3 text-sm font-medium">Columns</p>
      <p class="text-muted-foreground text-sm">Leave everything off for the default columns.</p>
      <div class="mt-1 flex flex-wrap gap-x-4 gap-y-1">
        <label v-for="f in (currentType?.fields ?? [])" :key="f.name" class="flex items-center gap-1.5 text-sm">
          <input type="checkbox" :checked="manageColumns.includes(f.name)" :disabled="manageBusy" @change="toggleColumn(f.name, ($event.target as HTMLInputElement).checked)" />
          {{ f.label }}
        </label>
      </div>
      <p v-if="manageFailed" class="text-destructive mt-2 text-sm" role="alert">{{ manageFailed }}</p>
      <template #footer>
        <ActionButton kind="primary" density="row" label="Save list" :loading="manageBusy" @press="saveManaged" />
      </template>
    </ResponsiveDialog>

    <ResponsiveDialog v-model:open="queueOpen" title="Queue messages" :wide="true" :dismissible="!queueBusy">
      <div class="mb-3 flex items-center gap-4" role="radiogroup" aria-label="Channel">
        <label class="flex items-center gap-1.5 text-sm">
          <input v-model="queueChannel" type="radio" value="email" :disabled="queueBusy" /> Email
        </label>
        <label class="flex items-center gap-1.5 text-sm">
          <input v-model="queueChannel" type="radio" value="message" :disabled="queueBusy" /> Message
        </label>
      </div>
      <p class="text-sm font-medium">Audience exclusion <span class="text-muted-foreground font-normal">(optional)</span></p>
      <p class="text-muted-foreground mb-2 text-sm">Rows matching this stay out of the send, for example invited guests who never responded.</p>
      <ConditionBuilder v-model="queueAudience" :fields="builderFields" />
      <div v-if="!queuePreview" class="mt-3">
        <ActionButton kind="secondary" density="row" label="Preview counts" :loading="queueBusy" @press="previewQueue" />
      </div>
      <div v-else class="bg-muted mt-3 rounded-md p-3 text-sm">
        <p><strong>{{ queuePreview.queued }}</strong> of {{ queuePreview.total }} matching rows will be queued.</p>
        <p v-if="queuePreview.excluded > 0n" class="text-muted-foreground">{{ queuePreview.excluded }} excluded by the audience rule.</p>
        <p class="text-muted-foreground">Nothing sends in beta; the Jobs page shows the queued rows.</p>
      </div>
      <p v-if="queueFailed" class="text-destructive mt-2 text-sm" role="alert">{{ queueFailed }}</p>
      <template #footer>
        <ActionButton kind="primary" density="row" :label="queuePreview ? `Queue ${queuePreview.queued}` : 'Queue'" :loading="queueBusy" :disabled="!queuePreview" @press="confirmQueue" />
      </template>
    </ResponsiveDialog>

    <ConfirmDialog
      :open="confirmDeleteOpen"
      title="Delete this list?"
      body="Saved lists cannot be recovered. Records stay untouched."
      confirm-label="Delete list"
      cancel-label="Keep it"
      confirm-icon="trash-2"
      @confirm="confirmDelete"
      @update:open="(value: boolean) => { if (!value) confirmDeleteOpen = false }"
    />
  </div>
</template>
