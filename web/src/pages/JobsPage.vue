<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { eventClient, jobClient } from '../api'
import type { Event as BusEvent, Job, Occurrence, OutboxRow, Subscription } from '../gen/athena/v1/events_pb'
import ActionButton from '@/components/ActionButton.vue'
import DataTable, { type TableFilter } from '@/components/DataTable.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import PageHeader from '@/components/PageHeader.vue'
import PrimaryCell from '@/components/PrimaryCell.vue'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatWhen } from '../times'
import { Bell, CalendarDays, ListChecks, ScrollText, Send } from '@/icons'

// The Jobs page watches durable background work (ADR-0028, Proposed): jobs
// the runner claims with leases, committed events, outbox deliveries,
// schedule occurrences and declared subscriptions. Builders only: every call
// needs the build permission.

function describe(err: unknown, action: string): string {
  if (!(err instanceof ConnectError)) return `Could not ${action}.`
  if (err.code === Code.PermissionDenied) return `You do not have permission to ${action}.`
  return `Could not ${action}: ${err.rawMessage}`
}

function short(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id
}

function pretty(raw: string): string {
  if (!raw) return '—'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

type Tone = 'secondary' | 'outline' | 'destructive'

function tone(status: string): Tone {
  if (status === 'dead') return 'destructive'
  if (status === 'succeeded' || status === 'dispatched' || status === 'done') return 'secondary'
  return 'outline'
}

const tab = ref('jobs')

// Jobs: server keyset paging with a server status filter. The filter keeps
// one status: the API reads one, so picking another replaces it.
const jobs = ref<Job[]>([])
const jobsLoading = ref(false)
const jobsError = ref<string | null>(null)
const jobsPage = ref(1)
const jobsHasNext = ref(false)
const jobsTokens = ref<string[]>([''])
const jobsPageSize = ref(25)
const jobsStatusSelected = ref<string[]>([])
let jobsRequest = 0

const jobsPaging = computed(() => ({ page: jobsPage.value, total: null as number | null, hasNext: jobsHasNext.value }))
const jobsFilters = computed<TableFilter[]>(() => [
  {
    id: 'status', label: 'Status',
    options: ['queued', 'leased', 'succeeded', 'dead'].map((status) => ({ value: status, label: status })),
    selected: jobsStatusSelected.value,
  },
])

async function loadJobs(reset = true) {
  if (reset) {
    jobsPage.value = 1
    jobsTokens.value = ['']
  }
  const request = ++jobsRequest
  jobsLoading.value = true
  jobsError.value = null
  try {
    const res = await jobClient.listJobs({
      status: jobsStatusSelected.value[0] ?? '',
      pageSize: jobsPageSize.value,
      pageToken: jobsTokens.value[jobsPage.value - 1] ?? '',
    })
    if (request !== jobsRequest) return
    jobs.value = res.jobs
    jobsHasNext.value = res.nextPageToken !== ''
    if (res.nextPageToken && jobsPage.value === jobsTokens.value.length) jobsTokens.value.push(res.nextPageToken)
  } catch (err) {
    if (request !== jobsRequest) return
    jobsError.value = describe(err, 'load the jobs')
  } finally {
    if (request === jobsRequest) jobsLoading.value = false
  }
}

function jobsPrevPage() {
  if (jobsPage.value <= 1) return
  jobsPage.value -= 1
  void loadJobs(false)
}

function jobsNextPage() {
  if (!jobsHasNext.value) return
  jobsPage.value += 1
  void loadJobs(false)
}

function onJobsFiltersChanged(next: TableFilter[]) {
  const selected = next.find((filter) => filter.id === 'status')?.selected ?? []
  jobsStatusSelected.value = selected.length > 1 ? selected.slice(-1) : selected
  void loadJobs(true)
}

const jobColumns = [
  {
    id: 'job', header: 'Job', accessorFn: (job: Job) => job.kind, meta: { label: 'Job' },
    cell: ({ row }: { row: { original: Job } }) => h(PrimaryCell, { title: row.original.kind, subtitle: short(row.original.id) }),
  },
  {
    id: 'status', header: 'Status', accessorFn: (job: Job) => job.status, meta: { label: 'Status' },
    cell: ({ row }: { row: { original: Job } }) => h(Badge, { variant: tone(row.original.status) }, () => row.original.status),
  },
  {
    id: 'attempts', header: 'Attempts', accessorFn: (job: Job) => job.attempts, meta: { label: 'Attempts', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: Job } }) => `${row.original.attempts} of ${row.original.maxAttempts}`,
  },
  {
    id: 'updated', header: 'Updated', accessorFn: (job: Job) => job.updatedAt, meta: { label: 'Updated', hideOnMobile: true },
    cell: ({ row }: { row: { original: Job } }) => formatWhen(row.original.updatedAt),
  },
]

const selectedJob = ref<Job | null>(null)
const retryingJob = ref(false)

// The list carries no payload or result, so opening a row fills them from
// GetJob. The sheet opens at once on the listed row.
async function openJob(job: Job) {
  selectedJob.value = job
  try {
    const res = await jobClient.getJob({ id: job.id })
    if (selectedJob.value?.id === job.id && res.job) selectedJob.value = res.job
  } catch (err) {
    toast.error(describe(err, 'load the job'))
  }
}

async function retryJob(job: Job) {
  retryingJob.value = true
  try {
    const res = await jobClient.retryJob({ id: job.id })
    toast.success(`Queued ${job.kind} again as ${short(res.jobId)}.`)
    await loadJobs(false)
    if (selectedJob.value?.id === job.id) {
      try {
        selectedJob.value = (await jobClient.getJob({ id: job.id })).job ?? selectedJob.value
      } catch {
        selectedJob.value = null
      }
    }
  } catch (err) {
    toast.error(describe(err, 'retry the job'))
  } finally {
    retryingJob.value = false
  }
}

// Events: committed envelopes, newest first, server-paged. Types are
// open-ended (Packs declare their own), so the tab carries no filter.
const events = ref<BusEvent[]>([])
const eventsLoading = ref(false)
const eventsError = ref<string | null>(null)
const eventsPage = ref(1)
const eventsHasNext = ref(false)
const eventsTokens = ref<string[]>([''])
const eventsPageSize = ref(25)
let eventsRequest = 0

const eventsPaging = computed(() => ({ page: eventsPage.value, total: null as number | null, hasNext: eventsHasNext.value }))

async function loadEvents(reset = true) {
  if (reset) {
    eventsPage.value = 1
    eventsTokens.value = ['']
  }
  const request = ++eventsRequest
  eventsLoading.value = true
  eventsError.value = null
  try {
    const res = await eventClient.listEvents({ pageSize: eventsPageSize.value, pageToken: eventsTokens.value[eventsPage.value - 1] ?? '' })
    if (request !== eventsRequest) return
    events.value = res.events
    eventsHasNext.value = res.nextPageToken !== ''
    if (res.nextPageToken && eventsPage.value === eventsTokens.value.length) eventsTokens.value.push(res.nextPageToken)
  } catch (err) {
    if (request !== eventsRequest) return
    eventsError.value = describe(err, 'load the events')
  } finally {
    if (request === eventsRequest) eventsLoading.value = false
  }
}

function eventsPrevPage() {
  if (eventsPage.value <= 1) return
  eventsPage.value -= 1
  void loadEvents(false)
}

function eventsNextPage() {
  if (!eventsHasNext.value) return
  eventsPage.value += 1
  void loadEvents(false)
}

const eventColumns = [
  {
    id: 'event', header: 'Event', accessorFn: (event: BusEvent) => event.eventType, meta: { label: 'Event' },
    cell: ({ row }: { row: { original: BusEvent } }) => h(PrimaryCell, { title: row.original.eventType, subtitle: `${row.original.subjectType} ${row.original.subjectId}` }),
  },
  {
    id: 'sequence', header: 'Sequence', accessorFn: (event: BusEvent) => Number(event.sequence), meta: { label: 'Sequence', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: BusEvent } }) => String(row.original.sequence),
  },
  {
    id: 'actor', header: 'Actor', accessorFn: (event: BusEvent) => event.actor, meta: { label: 'Actor', hideOnMobile: true },
    cell: ({ row }: { row: { original: BusEvent } }) => row.original.actor || '—',
  },
  {
    id: 'when', header: 'When', accessorFn: (event: BusEvent) => event.createdAt, meta: { label: 'When' },
    cell: ({ row }: { row: { original: BusEvent } }) => formatWhen(row.original.createdAt),
  },
]

const selectedEvent = ref<BusEvent | null>(null)

// Outbox: deliveries on their way to consumers, server-paged with a server
// status filter. Dead rows can be queued again as fresh events.
const outbox = ref<OutboxRow[]>([])
const outboxLoading = ref(false)
const outboxError = ref<string | null>(null)
const outboxPage = ref(1)
const outboxHasNext = ref(false)
const outboxTokens = ref<string[]>([''])
const outboxPageSize = ref(25)
const outboxStatusSelected = ref<string[]>([])
let outboxRequest = 0

const outboxPaging = computed(() => ({ page: outboxPage.value, total: null as number | null, hasNext: outboxHasNext.value }))
const outboxFilters = computed<TableFilter[]>(() => [
  {
    id: 'status', label: 'Status',
    options: ['pending', 'leased', 'dispatched', 'dead'].map((status) => ({ value: status, label: status })),
    selected: outboxStatusSelected.value,
  },
])

async function loadOutbox(reset = true) {
  if (reset) {
    outboxPage.value = 1
    outboxTokens.value = ['']
  }
  const request = ++outboxRequest
  outboxLoading.value = true
  outboxError.value = null
  try {
    const res = await eventClient.listOutbox({
      status: outboxStatusSelected.value[0] ?? '',
      pageSize: outboxPageSize.value,
      pageToken: outboxTokens.value[outboxPage.value - 1] ?? '',
    })
    if (request !== outboxRequest) return
    outbox.value = res.rows
    outboxHasNext.value = res.nextPageToken !== ''
    if (res.nextPageToken && outboxPage.value === outboxTokens.value.length) outboxTokens.value.push(res.nextPageToken)
  } catch (err) {
    if (request !== outboxRequest) return
    outboxError.value = describe(err, 'load the deliveries')
  } finally {
    if (request === outboxRequest) outboxLoading.value = false
  }
}

function outboxPrevPage() {
  if (outboxPage.value <= 1) return
  outboxPage.value -= 1
  void loadOutbox(false)
}

function outboxNextPage() {
  if (!outboxHasNext.value) return
  outboxPage.value += 1
  void loadOutbox(false)
}

function onOutboxFiltersChanged(next: TableFilter[]) {
  const selected = next.find((filter) => filter.id === 'status')?.selected ?? []
  outboxStatusSelected.value = selected.length > 1 ? selected.slice(-1) : selected
  void loadOutbox(true)
}

const outboxColumns = [
  {
    id: 'delivery', header: 'Delivery', accessorFn: (row: OutboxRow) => row.eventType, meta: { label: 'Delivery' },
    cell: ({ row }: { row: { original: OutboxRow } }) => h(PrimaryCell, { title: row.original.eventType, subtitle: short(row.original.id) }),
  },
  {
    id: 'status', header: 'Status', accessorFn: (row: OutboxRow) => row.status, meta: { label: 'Status' },
    cell: ({ row }: { row: { original: OutboxRow } }) => h(Badge, { variant: tone(row.original.status) }, () => row.original.status),
  },
  {
    id: 'attempts', header: 'Attempts', accessorFn: (row: OutboxRow) => row.attempts, meta: { label: 'Attempts', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: OutboxRow } }) => String(row.original.attempts),
  },
  {
    id: 'error', header: 'Last error', accessorFn: (row: OutboxRow) => row.lastError, meta: { label: 'Last error', hideOnMobile: true },
    cell: ({ row }: { row: { original: OutboxRow } }) => row.original.lastError || '—',
  },
]

const selectedRow = ref<OutboxRow | null>(null)
const requeueingRow = ref(false)

async function requeueRow(row: OutboxRow) {
  requeueingRow.value = true
  try {
    const res = await eventClient.requeueOutbox({ id: row.id })
    toast.success(`Queued ${row.eventType} again as ${short(res.eventId)}.`)
    await loadOutbox(false)
  } catch (err) {
    toast.error(describe(err, 'queue the delivery again'))
  } finally {
    requeueingRow.value = false
  }
}

// Schedules: one row per scheduled period, a bounded list with client search
// and a client status filter.
const occurrences = ref<Occurrence[]>([])
const occurrencesLoading = ref(false)
const occurrencesError = ref<string | null>(null)
const occurrenceSearch = ref('')
const occurrenceStatusSelected = ref<string[]>([])

const occurrenceFilters = computed<TableFilter[]>(() => [
  {
    id: 'status', label: 'Status',
    options: ['pending', 'leased', 'done', 'skipped', 'dead'].map((status) => ({ value: status, label: status })),
    selected: occurrenceStatusSelected.value,
  },
])

const occurrenceFilteredActive = computed(() => occurrenceSearch.value.trim() !== '' || occurrenceStatusSelected.value.length > 0)

const filteredOccurrences = computed(() => {
  const needle = occurrenceSearch.value.trim().toLowerCase()
  return occurrences.value.filter((occurrence) => {
    if (occurrenceStatusSelected.value.length > 0 && !occurrenceStatusSelected.value.includes(occurrence.status)) return false
    return !needle || `${occurrence.schedule} ${occurrence.period} ${occurrence.scope}`.toLowerCase().includes(needle)
  })
})

function onOccurrenceFiltersChanged(next: TableFilter[]) {
  occurrenceStatusSelected.value = next.find((filter) => filter.id === 'status')?.selected ?? []
}

async function loadOccurrences() {
  occurrencesLoading.value = true
  occurrencesError.value = null
  try {
    occurrences.value = (await jobClient.listOccurrences({ limit: 100 })).occurrences
  } catch (err) {
    occurrencesError.value = describe(err, 'load the schedules')
  } finally {
    occurrencesLoading.value = false
  }
}

const occurrenceColumns = [
  {
    id: 'schedule', header: 'Schedule', accessorFn: (occurrence: Occurrence) => occurrence.schedule, meta: { label: 'Schedule' },
    cell: ({ row }: { row: { original: Occurrence } }) => h(PrimaryCell, { title: row.original.schedule, subtitle: row.original.scope || null }),
  },
  {
    id: 'period', header: 'Period', accessorFn: (occurrence: Occurrence) => occurrence.period, meta: { label: 'Period' },
    cell: ({ row }: { row: { original: Occurrence } }) => row.original.period,
  },
  {
    id: 'status', header: 'Status', accessorFn: (occurrence: Occurrence) => occurrence.status, meta: { label: 'Status' },
    cell: ({ row }: { row: { original: Occurrence } }) => h(Badge, { variant: tone(row.original.status) }, () => row.original.status),
  },
  {
    id: 'job', header: 'Job', accessorFn: (occurrence: Occurrence) => occurrence.jobId, meta: { label: 'Job', hideOnMobile: true },
    cell: ({ row }: { row: { original: Occurrence } }) => (row.original.jobId ? short(row.original.jobId) : '—'),
  },
]

const selectedOccurrence = ref<Occurrence | null>(null)

// Subscriptions: the declared consumers, a small list with client search.
const subscriptions = ref<Subscription[]>([])
const subscriptionsLoading = ref(false)
const subscriptionsError = ref<string | null>(null)
const subscriptionSearch = ref('')

const filteredSubscriptions = computed(() => {
  const needle = subscriptionSearch.value.trim().toLowerCase()
  return subscriptions.value.filter((sub) => !needle || `${sub.consumer} ${sub.eventType} ${sub.action}`.toLowerCase().includes(needle))
})

async function loadSubscriptions() {
  subscriptionsLoading.value = true
  subscriptionsError.value = null
  try {
    subscriptions.value = (await eventClient.listSubscriptions({})).subscriptions
  } catch (err) {
    subscriptionsError.value = describe(err, 'load the subscriptions')
  } finally {
    subscriptionsLoading.value = false
  }
}

const subscriptionColumns = [
  {
    id: 'consumer', header: 'Consumer', accessorFn: (sub: Subscription) => sub.consumer, meta: { label: 'Consumer' },
    cell: ({ row }: { row: { original: Subscription } }) => h(PrimaryCell, { title: row.original.consumer, subtitle: row.original.pack || null }),
  },
  {
    id: 'type', header: 'Event type', accessorFn: (sub: Subscription) => sub.eventType, meta: { label: 'Event type' },
    cell: ({ row }: { row: { original: Subscription } }) => row.original.eventType,
  },
  {
    id: 'action', header: 'Action', accessorFn: (sub: Subscription) => sub.action, meta: { label: 'Action', hideOnMobile: true },
    cell: ({ row }: { row: { original: Subscription } }) => `${row.original.mode} · ${row.original.action}`,
  },
  {
    id: 'enabled', header: 'Enabled', accessorFn: (sub: Subscription) => (sub.enabled ? 1 : 0), meta: { label: 'Enabled', center: true },
    cell: ({ row }: { row: { original: Subscription } }) => (row.original.enabled ? 'Yes' : 'No'),
  },
]

const loadedTabs = new Set<string>()

function refresh() {
  if (tab.value === 'jobs') void loadJobs(true)
  else if (tab.value === 'events') void loadEvents(true)
  else if (tab.value === 'outbox') void loadOutbox(true)
  else if (tab.value === 'schedules') void loadOccurrences()
  else void loadSubscriptions()
}

function onTab(value: string | number) {
  tab.value = String(value)
  if (loadedTabs.has(tab.value)) return
  loadedTabs.add(tab.value)
  refresh()
}

onMounted(() => {
  loadedTabs.add('jobs')
  void loadJobs(true)
})
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Jobs" description="Background work: queued jobs, committed events, deliveries, schedules and subscriptions.">
      <template #actions>
        <ActionButton kind="secondary" label="Refresh" icon="refresh-ccw" @press="refresh" />
      </template>
    </PageHeader>
    <Tabs :model-value="tab" @update:model-value="onTab">
      <TabsList>
        <TabsTrigger value="jobs">Jobs</TabsTrigger>
        <TabsTrigger value="events">Events</TabsTrigger>
        <TabsTrigger value="outbox">Outbox</TabsTrigger>
        <TabsTrigger value="schedules">Schedules</TabsTrigger>
        <TabsTrigger value="subscriptions">Subscriptions</TabsTrigger>
      </TabsList>
      <TabsContent value="jobs">
        <DataTable
          :columns="jobColumns"
          :data="jobs"
          table-id="jobs-list"
          :get-row-id="(job) => job.id"
          :row-actions="(job) => job.status === 'dead' ? [{ id: 'retry', label: 'Retry', kind: 'secondary', icon: 'rotate-ccw', busy: retryingJob }] : []"
          :filters="jobsFilters"
          :server-paging="jobsPaging"
          :loading="jobsLoading"
          :error="jobsError"
          clickable
          :empty-icon="ListChecks"
          empty-title="No jobs yet"
          empty-body="Queued work will appear here with its results."
          @row-click="openJob"
          @row-action="(id, job) => { if (id === 'retry') void retryJob(job) }"
          @update:filters="onJobsFiltersChanged"
          @prev-page="jobsPrevPage"
          @next-page="jobsNextPage"
          @update:page-size="(size) => { jobsPageSize = size; void loadJobs(true) }"
          @retry="loadJobs(false)"
        />
      </TabsContent>
      <TabsContent value="events">
        <DataTable
          :columns="eventColumns"
          :data="events"
          table-id="events-list"
          :get-row-id="(event) => event.id"
          :server-paging="eventsPaging"
          :loading="eventsLoading"
          :error="eventsError"
          clickable
          :empty-icon="ScrollText"
          empty-title="No events yet"
          empty-body="Committed business events will appear here, newest first."
          @row-click="(event) => { selectedEvent = event }"
          @prev-page="eventsPrevPage"
          @next-page="eventsNextPage"
          @update:page-size="(size) => { eventsPageSize = size; void loadEvents(true) }"
          @retry="loadEvents(false)"
        />
      </TabsContent>
      <TabsContent value="outbox">
        <DataTable
          :columns="outboxColumns"
          :data="outbox"
          table-id="outbox-list"
          :get-row-id="(row) => row.id"
          :row-actions="(row) => row.status === 'dead' ? [{ id: 'requeue', label: 'Queue again', kind: 'secondary', icon: 'rotate-ccw', busy: requeueingRow }] : []"
          :filters="outboxFilters"
          :server-paging="outboxPaging"
          :loading="outboxLoading"
          :error="outboxError"
          clickable
          :empty-icon="Send"
          empty-title="No deliveries yet"
          empty-body="Events on their way to consumers will appear here."
          @row-click="(row) => { selectedRow = row }"
          @row-action="(id, row) => { if (id === 'requeue') void requeueRow(row) }"
          @update:filters="onOutboxFiltersChanged"
          @prev-page="outboxPrevPage"
          @next-page="outboxNextPage"
          @update:page-size="(size) => { outboxPageSize = size; void loadOutbox(true) }"
          @retry="loadOutbox(false)"
        />
      </TabsContent>
      <TabsContent value="schedules">
        <DataTable
          :columns="occurrenceColumns"
          :data="filteredOccurrences"
          table-id="schedules-list"
          :get-row-id="(occurrence) => `${occurrence.schedule} ${occurrence.scope} ${occurrence.period}`"
          v-model:search="occurrenceSearch"
          search-placeholder="Find schedules…"
          :filters="occurrenceFilters"
          :loading="occurrencesLoading"
          :error="occurrencesError"
          :filtered="occurrenceFilteredActive"
          clickable
          :empty-icon="CalendarDays"
          empty-title="No schedules yet"
          empty-body="Scheduled periods will appear here in period order."
          @row-click="(occurrence) => { selectedOccurrence = occurrence }"
          @update:filters="onOccurrenceFiltersChanged"
          @retry="loadOccurrences"
        />
      </TabsContent>
      <TabsContent value="subscriptions">
        <DataTable
          :columns="subscriptionColumns"
          :data="filteredSubscriptions"
          table-id="subscriptions-list"
          :get-row-id="(sub) => `${sub.consumer} ${sub.eventType}`"
          v-model:search="subscriptionSearch"
          search-placeholder="Find subscriptions…"
          :loading="subscriptionsLoading"
          :error="subscriptionsError"
          :filtered="subscriptionSearch.trim() !== ''"
          :empty-icon="Bell"
          empty-title="No subscriptions yet"
          empty-body="Consumers declare their interest in event types here."
          @retry="loadSubscriptions"
        />
      </TabsContent>
    </Tabs>
    <DetailSheet
      v-if="selectedJob"
      :open="true"
      title="Job"
      @update:open="(value: boolean) => { if (!value) selectedJob = null }"
    >
      <template #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ selectedJob.id }}</p>
      </template>
      <template #status>
        <Badge :variant="tone(selectedJob.status)">{{ selectedJob.status }}</Badge>
      </template>
      <div class="sheet-section">
        <p class="sheet-label">Work</p>
        <p class="sheet-lead">{{ selectedJob.kind }}</p>
        <p class="muted-label">{{ selectedJob.attempts }} of {{ selectedJob.maxAttempts }} attempts · updated {{ formatWhen(selectedJob.updatedAt) }}</p>
      </div>
      <div v-if="selectedJob.lastError" class="sheet-section">
        <p class="sheet-label">Last error</p>
        <p>{{ selectedJob.lastError }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Lease and owner</p>
        <p v-if="selectedJob.leaseOwner" class="muted-label">Held by {{ selectedJob.leaseOwner }} until {{ formatWhen(selectedJob.leaseExpiresAt) }}</p>
        <p v-else class="muted-label">No lease held.</p>
        <p v-if="selectedJob.ownerUserId || selectedJob.ownerRole" class="muted-label">Owner: {{ selectedJob.ownerUserId }} {{ selectedJob.ownerRole }}</p>
        <p v-if="selectedJob.occurrenceKey" class="muted-label font-mono">{{ selectedJob.occurrenceKey }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Payload</p>
        <pre class="font-mono text-xs whitespace-pre-wrap break-all">{{ pretty(selectedJob.payload) }}</pre>
      </div>
      <div v-if="selectedJob.result" class="sheet-section">
        <p class="sheet-label">Result</p>
        <pre class="font-mono text-xs whitespace-pre-wrap break-all">{{ pretty(selectedJob.result) }}</pre>
      </div>
      <template #footer>
        <ActionButton
          kind="secondary" label="Retry" icon="rotate-ccw" :busy="retryingJob"
          :disabled-reason="selectedJob.status === 'dead' ? null : 'Only dead work can be retried.'"
          @press="() => { if (selectedJob) void retryJob(selectedJob) }"
        />
      </template>
    </DetailSheet>
    <DetailSheet
      v-if="selectedEvent"
      :open="true"
      title="Event"
      @update:open="(value: boolean) => { if (!value) selectedEvent = null }"
    >
      <template #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ selectedEvent.id }}</p>
      </template>
      <template #status>
        <Badge variant="secondary">{{ selectedEvent.eventType }}</Badge>
      </template>
      <div class="sheet-section">
        <p class="sheet-label">What happened</p>
        <p class="sheet-lead">{{ selectedEvent.subjectType }} {{ selectedEvent.subjectId }}</p>
        <p class="muted-label">Revision {{ selectedEvent.subjectRevision }} · sequence {{ selectedEvent.sequence }} · {{ formatWhen(selectedEvent.createdAt) }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Cause</p>
        <p class="muted-label">By {{ selectedEvent.producer }}{{ selectedEvent.actor ? ` as ${selectedEvent.actor}` : '' }}</p>
        <p class="text-caption text-muted-foreground font-mono">{{ selectedEvent.correlationId }}</p>
        <p v-if="selectedEvent.causationId" class="muted-label">Caused by {{ short(selectedEvent.causationId) }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Payload</p>
        <pre class="font-mono text-xs whitespace-pre-wrap break-all">{{ pretty(selectedEvent.payload) }}</pre>
      </div>
    </DetailSheet>
    <DetailSheet
      v-if="selectedRow"
      :open="true"
      title="Delivery"
      @update:open="(value: boolean) => { if (!value) selectedRow = null }"
    >
      <template #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ selectedRow.id }}</p>
      </template>
      <template #status>
        <Badge :variant="tone(selectedRow.status)">{{ selectedRow.status }}</Badge>
      </template>
      <div class="sheet-section">
        <p class="sheet-label">Delivery</p>
        <p class="sheet-lead">{{ selectedRow.eventType }}</p>
        <p class="muted-label">{{ selectedRow.attempts }} attempts · {{ formatWhen(selectedRow.createdAt) }}</p>
      </div>
      <div v-if="selectedRow.lastError" class="sheet-section">
        <p class="sheet-label">Last error</p>
        <p>{{ selectedRow.lastError }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Payload</p>
        <pre class="font-mono text-xs whitespace-pre-wrap break-all">{{ pretty(selectedRow.payload) }}</pre>
      </div>
      <template #footer>
        <ActionButton
          kind="secondary" label="Queue again" icon="rotate-ccw" :busy="requeueingRow"
          :disabled-reason="selectedRow.status === 'dead' ? (selectedRow.eventId ? null : 'This delivery predates envelopes and cannot be queued again.') : 'Only dead deliveries can be queued again.'"
          @press="() => { if (selectedRow) void requeueRow(selectedRow) }"
        />
      </template>
    </DetailSheet>
    <DetailSheet
      v-if="selectedOccurrence"
      :open="true"
      title="Scheduled period"
      @update:open="(value: boolean) => { if (!value) selectedOccurrence = null }"
    >
      <template #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ selectedOccurrence.schedule }} · {{ selectedOccurrence.period }}</p>
      </template>
      <template #status>
        <Badge :variant="tone(selectedOccurrence.status)">{{ selectedOccurrence.status }}</Badge>
      </template>
      <div class="sheet-section">
        <p class="sheet-label">Schedule</p>
        <p class="sheet-lead">{{ selectedOccurrence.schedule }}</p>
        <p class="muted-label">{{ selectedOccurrence.timezone }} · catches up by {{ selectedOccurrence.catchupPolicy === 'run' ? 'running late' : 'skipping' }}</p>
      </div>
      <div v-if="selectedOccurrence.result" class="sheet-section">
        <p class="sheet-label">Result</p>
        <pre class="font-mono text-xs whitespace-pre-wrap break-all">{{ pretty(selectedOccurrence.result) }}</pre>
      </div>
    </DetailSheet>
  </div>
</template>
