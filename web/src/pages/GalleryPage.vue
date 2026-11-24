// Gallery shows every shared composition in every state. It is a Build page
// for designers and developers; it stays out of the nav and loads by address
// (?mode=build&page=gallery, /build/gallery).
<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { CalendarDays, Inbox } from '@/icons'

import { toast } from 'vue-sonner'
import ActionButton from '@/components/ActionButton.vue'
import ActionGroup from '@/components/ActionGroup.vue'
import type { ActionItem } from '@/components/actions'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ConditionBuilder from '@/components/conditions/ConditionBuilder.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import DataTable, { type TableFilter } from '@/components/DataTable.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import FieldRow from '@/components/FieldRow.vue'
import HelpHint from '@/copy/HelpHint.vue'
import FormSection from '@/components/FormSection.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import PageHeader from '@/components/PageHeader.vue'
import PermissionDenied from '@/components/PermissionDenied.vue'
import PrimaryCell from '@/components/PrimaryCell.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import type { BuilderField, ConditionTree } from '@/conditions'

const emit = defineEmits<{ goHome: [] }>()

const dangerOpen = ref(false)
const plainOpen = ref(false)
const busyOpen = ref(false)
const busyWorking = ref(false)
const disabledOpen = ref(false)
const sheetOpen = ref(false)
const wideSheetOpen = ref(false)
const galleryName = ref('')
const demoBuilderFields: BuilderField[] = [
  { name: 'doc.name', label: 'Name', type: 'Text', kind: 'string' },
  { name: 'doc.qty', label: 'Qty', type: 'Number', kind: 'decimal' },
  { name: 'doc.due', label: 'Due', type: 'Date', kind: 'date' },
]
const demoBuilderTree = ref<ConditionTree>({ root: { all: true, conditions: [{ field: 'doc.qty', op: 'more_than', values: ['0'] }] } })
const responsiveOpen = ref(false)
const responsiveNote = ref('')

const demoActions: ActionItem[] = [
  { id: 'assign', label: 'Assign Role', kind: 'primary', icon: 'plus' },
  { id: 'resend', label: 'Resend invite', kind: 'secondary', icon: 'send' },
  { id: 'reset', label: 'Reset', kind: 'secondary', icon: 'key-round' },
  { id: 'suspend', label: 'Suspend', kind: 'caution', icon: 'pause' },
  { id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2' },
]
const demoRestore: ActionItem[] = [
  { id: 'reactivate', label: 'Reactivate', kind: 'restore', icon: 'play' },
]

function confirmBusy() {
  busyWorking.value = true
  window.setTimeout(() => {
    busyWorking.value = false
    busyOpen.value = false
    toast.success('Record deleted (demo).')
  }, 1500)
}

type DemoCustomer = { id: string; name: string; email: string; status: string; plan: string; seats: number; updated: string }
const firstNames = ['Ada', 'Grace', 'Alan', 'Linus', 'Margaret', 'Dennis', 'Barbara', 'Ken']
const lastNames = ['Lovelace', 'Hopper', 'Turing', 'Oak']
const demoStatuses = ['Active', 'Pending', 'Inactive']
const demoPlans = ['Free', 'Team', 'Business']
function makeCustomers(): DemoCustomer[] {
  return Array.from({ length: 32 }, (_, index) => {
    const name = `${firstNames[index % firstNames.length]} ${lastNames[Math.floor(index / firstNames.length) % lastNames.length]}`
    return {
      id: `customer-${index + 1}`,
      name,
      email: `${name.toLowerCase().replace(/[^a-z]+/g, '.')}@example.com`,
      status: demoStatuses[index % demoStatuses.length]!,
      plan: demoPlans[(index * 2 + 1) % demoPlans.length]!,
      seats: (index * 7) % 40 + 1,
      updated: `0${(index % 9) + 1} Nov 2026`,
    }
  })
}
const demoColumns = [
  {
    accessorKey: 'name',
    header: 'Customer',
    meta: { label: 'Customer' },
    cell: ({ row }: { row: { original: DemoCustomer } }) => h(PrimaryCell, { title: row.original.name, subtitle: row.original.email, icon: 'user' }),
  },
  {
    accessorKey: 'status',
    header: 'Status',
    meta: { label: 'Status' },
    cell: ({ row }: { row: { original: DemoCustomer } }) => h(StatusBadge, { status: row.original.status.toLowerCase(), label: row.original.status }),
  },
  { accessorKey: 'plan', header: 'Plan', meta: { label: 'Plan', hideOnMobile: true } },
  { accessorKey: 'seats', header: 'Seats', meta: { label: 'Seats', numeric: true } },
  { accessorKey: 'updated', header: 'Updated', meta: { label: 'Updated', hideOnMobile: true } },
]
const demoRows = ref<DemoCustomer[]>(makeCustomers())
const demoSearch = ref('')
const demoSelected = ref<string[]>([])
const demoFilters = ref<TableFilter[]>([
  { id: 'status', label: 'Status', options: demoStatuses.map((status) => ({ value: status, label: status })), selected: [] },
  { id: 'plan', label: 'Plan', options: demoPlans.map((plan) => ({ value: plan, label: plan })), selected: [] },
])
const demoFiltered = computed(() => {
  const needle = demoSearch.value.trim().toLowerCase()
  const statuses = demoFilters.value.find((filter) => filter.id === 'status')?.selected ?? []
  const plans = demoFilters.value.find((filter) => filter.id === 'plan')?.selected ?? []
  return demoRows.value.filter((row) => {
    if (statuses.length > 0 && !statuses.includes(row.status)) return false
    if (plans.length > 0 && !plans.includes(row.plan)) return false
    return !needle || `${row.name} ${row.email}`.toLowerCase().includes(needle)
  })
})
const demoFilteredActive = computed(() => demoSearch.value.trim() !== '' || demoFilters.value.some((filter) => filter.selected.length > 0))
const tableLoading = ref(false)
const tableError = ref<string | null>(null)

function demoBulk(id: string) {
  if (id === 'export') {
    toast.success(`Exported ${demoSelected.value.length} customers (demo).`)
    demoSelected.value = []
  } else if (id === 'delete') {
    demoRows.value = demoRows.value.filter((row) => !demoSelected.value.includes(row.id))
    toast.success('Customers deleted (demo).')
    demoSelected.value = []
  }
}

function demoRowAction(id: string, row: DemoCustomer) {
  if (id === 'view') toast.info(`${row.name} (demo).`)
  else if (id === 'delete') {
    demoRows.value = demoRows.value.filter((item) => item.id !== row.id)
    toast.success(`${row.name} deleted (demo).`)
  }
}

// A fixed keyset simulation: 90 rows, no total, forward tokens only.
const serverRows = Array.from({ length: 90 }, (_, index) => ({ id: `txn-${index + 1}`, name: `Payment ${index + 1}`, amount: (index * 137) % 900 + 50 }))
const serverPage = ref(1)
const serverSize = ref(25)
const serverSlice = computed(() => serverRows.slice((serverPage.value - 1) * serverSize.value, serverPage.value * serverSize.value))
const serverPaging = computed(() => ({ page: serverPage.value, total: null as number | null, hasNext: serverPage.value * serverSize.value < serverRows.length }))
const serverColumns = [
  { accessorKey: 'name', header: 'Payment', meta: { label: 'Payment' } },
  { accessorKey: 'amount', header: 'Amount', meta: { label: 'Amount', numeric: true } },
]

const statuses = ['draft', 'pending', 'submitted', 'active', 'paid', 'open', 'inactive', 'cancelled', 'failed', 'overdue', 'archived']

const pickedLabel = ref('Pick a date')

function onPick(next: unknown) {
  if (next && typeof next === 'object' && !Array.isArray(next) && 'toString' in next) pickedLabel.value = String(next)
}
</script>

<template>
  <div class="mx-auto w-full max-w-3xl space-y-10">
    <PageHeader title="Gallery" description="Every shared composition in every state.">
      <template #actions>
        <ActionButton kind="secondary" label="Back to Overview" icon="house" @press="emit('goHome')" />
      </template>
    </PageHeader>

    <section aria-labelledby="gal-actions">
      <h2 id="gal-actions" class="text-lg font-medium">ActionButton</h2>
      <p class="text-muted-foreground mt-1 text-sm">One labelled action in six kinds, five densities, with busy and disabled-with-reason states. Hover the disabled one for its reason.</p>
      <div class="mt-4 space-y-4 rounded-[var(--radius)] border p-4">
        <div class="flex flex-wrap items-center gap-2">
          <ActionButton kind="primary" label="Invite user" icon="plus" @press="toast.info('Invite (demo).')" />
          <ActionButton kind="secondary" label="Edit" icon="pencil" @press="toast.info('Edit (demo).')" />
          <ActionButton kind="quiet" label="View" icon="eye" @press="toast.info('View (demo).')" />
          <ActionButton kind="caution" label="Suspend" icon="pause" @press="toast.info('Suspend (demo).')" />
          <ActionButton kind="destructive" label="Delete" icon="trash-2" @press="toast.info('Delete (demo).')" />
          <ActionButton kind="restore" label="Reactivate" icon="play" @press="toast.info('Reactivate (demo).')" />
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <ActionButton kind="secondary" density="page" label="Page" icon="plus" />
          <ActionButton kind="secondary" density="section" label="Section" icon="plus" />
          <ActionButton kind="secondary" density="row" label="Row" icon="plus" />
          <ActionButton kind="secondary" density="inline" label="Inline" icon="plus" />
          <ActionButton kind="secondary" density="touch" label="Touch" icon="plus" />
          <ActionButton kind="quiet" density="row" icon-only label="More actions" icon="ellipsis" />
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <ActionButton kind="primary" label="Saving" icon="save" busy />
          <ActionButton kind="secondary" label="Delete" icon="trash-2" disabled-reason="Close its open jobs first." />
          <ActionButton kind="secondary" label="Inbox" icon="inbox" counter="12" shortcut="⌘1" helper="New" />
        </div>
        <ActionButton kind="primary" label="Full-width in drawers" icon="plus" full-bleed @press="toast.info('Full bleed (demo).')" />
      </div>
    </section>

    <section aria-labelledby="gal-actiongroup">
      <h2 id="gal-actiongroup" class="text-lg font-medium">ActionGroup</h2>
      <p class="text-muted-foreground mt-1 text-sm">Actions grouped by kind: close within a kind, wider between kinds, a divider before destructive. Below 640 px the stacked group becomes full-width buttons.</p>
      <div class="mt-4 space-y-4 rounded-[var(--radius)] border p-4">
        <ActionGroup :actions="demoActions" aria-label="User actions" @press="(id) => toast.info(`${id} (demo).`)" />
        <ActionGroup :actions="demoRestore" stack-on-phones aria-label="Restore actions" @press="(id) => toast.info(`${id} (demo).`)" />
      </div>
    </section>

    <section aria-labelledby="gal-pageheader">
      <h2 id="gal-pageheader" class="text-lg font-medium">PageHeader</h2>
      <p class="text-muted-foreground mt-1 text-sm">Title, one line of purpose, optional actions. This page's own header is the with-actions state.</p>
      <div class="mt-4 rounded-[var(--radius)] border p-4">
        <PageHeader title="Without actions" description="Some pages need nothing on the right." />
      </div>
    </section>

    <section aria-labelledby="gal-helphint">
      <h2 id="gal-helphint" class="text-lg font-medium">HelpHint</h2>
      <p class="text-muted-foreground mt-1 text-sm">One glossary term explained next to a section title: a popover on desktop, a drawer on touch. Never inside a sentence.</p>
      <div class="mt-4 rounded-[var(--radius)] border p-4">
        <p class="text-sm font-medium">Period close <HelpHint term="hardClose" /></p>
        <p class="text-muted-foreground mt-1 text-sm">Nothing can be recorded or changed here again.</p>
      </div>
    </section>

    <section aria-labelledby="gal-emptystate">
      <h2 id="gal-emptystate" class="text-lg font-medium">EmptyState</h2>
      <p class="text-muted-foreground mt-1 text-sm">Nothing here yet: figure or icon, title, one helpful line, optional action.</p>
      <div class="mt-4 grid grid-cols-1 gap-4 min-[640px]:grid-cols-2">
        <div class="rounded-[var(--radius)] border">
          <EmptyState :icon="Inbox" title="All caught up" body="Records assigned to you will show up here." />
        </div>
        <div class="rounded-[var(--radius)] border">
          <EmptyState art="empty-reports" title="No reports yet" body="Build a report to rerun a list or total any time.">
            <template #action>
              <ActionButton kind="primary" density="row" label="New report" icon="plus" @press="toast.info('New report (demo).')" />
            </template>
          </EmptyState>
        </div>
      </div>
    </section>

    <section aria-labelledby="gal-errorstate">
      <h2 id="gal-errorstate" class="text-lg font-medium">ErrorState</h2>
      <p class="text-muted-foreground mt-1 text-sm">A page that failed, with a retry.</p>
      <div class="mt-4 grid grid-cols-1 gap-4 min-[640px]:grid-cols-2">
        <div class="rounded-[var(--radius)] border">
          <ErrorState @retry="toast.info('Retry (demo).')" />
        </div>
        <div class="rounded-[var(--radius)] border">
          <ErrorState title="Records did not load" message="The server took too long. Try again." @retry="toast.info('Retry (demo).')" />
        </div>
      </div>
    </section>

    <section aria-labelledby="gal-permissiondenied">
      <h2 id="gal-permissiondenied" class="text-lg font-medium">PermissionDenied</h2>
      <p class="text-muted-foreground mt-1 text-sm">A page the user must not see. The button below goes home for real.</p>
      <div class="mt-4 rounded-[var(--radius)] border">
        <PermissionDenied page-title="Ledger" @go-home="emit('goHome')" />
      </div>
    </section>

    <section aria-labelledby="gal-loading">
      <h2 id="gal-loading" class="text-lg font-medium">LoadingSkeleton</h2>
      <p class="text-muted-foreground mt-1 text-sm">Holds the page shape while its code loads.</p>
      <div class="mt-4 grid grid-cols-1 gap-4 min-[640px]:grid-cols-2">
        <div class="rounded-[var(--radius)] border p-4">
          <LoadingSkeleton />
        </div>
        <div class="rounded-[var(--radius)] border p-4">
          <LoadingSkeleton :rows="5" />
        </div>
      </div>
    </section>

    <section aria-labelledby="gal-confirm">
      <h2 id="gal-confirm" class="text-lg font-medium">ConfirmDialog</h2>
      <p class="text-muted-foreground mt-1 text-sm">Asks before a destructive action, with busy and disabled-with-reason states. Opens from the buttons below.</p>
      <div class="mt-4 flex flex-wrap gap-2">
        <ActionButton kind="destructive" label="Delete record…" icon="trash-2" @press="dangerOpen = true" />
        <ActionButton kind="secondary" label="Discard draft…" icon="x" @press="plainOpen = true" />
        <ActionButton kind="destructive" label="Delete with busy…" icon="trash-2" @press="busyOpen = true" />
        <ActionButton kind="destructive" label="Delete disabled…" icon="trash-2" @press="disabledOpen = true" />
      </div>
      <ConfirmDialog
        v-model:open="dangerOpen"
        title="Delete this record?"
        body="Its values leave every view at once. This cannot be undone."
        confirm-label="Delete"
        @confirm="dangerOpen = false; toast.success('Record deleted (demo).')"
      />
      <ConfirmDialog
        v-model:open="plainOpen"
        title="Discard this draft?"
        body="Your edits since the last save go away."
        confirm-label="Discard"
        cancel-label="Keep editing"
        confirm-icon="x"
        :danger="false"
        @confirm="plainOpen = false; toast.info('Draft discarded (demo).')"
      />
      <ConfirmDialog
        v-model:open="busyOpen"
        title="Delete this record?"
        body="Its values leave every view at once. This cannot be undone."
        confirm-label="Delete"
        :busy="busyWorking"
        @confirm="confirmBusy"
      />
      <ConfirmDialog
        v-model:open="disabledOpen"
        title="Delete this record?"
        body="Its values leave every view at once. This cannot be undone."
        confirm-label="Delete"
        disabled-reason="Close its open jobs first."
        @confirm="disabledOpen = false"
      />
    </section>

    <section aria-labelledby="gal-formsection">
      <h2 id="gal-formsection" class="text-lg font-medium">FormSection</h2>
      <p class="text-muted-foreground mt-1 text-sm">One idea per card: a title, one line, the content.</p>
      <div class="mt-4">
        <FormSection title="Workspace" description="The name people see in the sidebar.">
          <FieldRow label="Name" for="gallery-section-name">
            <Input id="gallery-section-name" value="Acme" />
          </FieldRow>
        </FormSection>
      </div>
    </section>

    <section aria-labelledby="gal-fieldrow">
      <h2 id="gal-fieldrow" class="text-lg font-medium">FieldRow</h2>
      <p class="text-muted-foreground mt-1 text-sm">Label left, control right on desktop; stacked on phones or on request.</p>
      <div class="mt-4 rounded-[var(--radius)] border p-4">
        <div class="space-y-4">
          <FieldRow label="Workspace name" hint="Shown in the sidebar and the sign-in card.">
            <Input id="gallery-name" v-model="galleryName" placeholder="Acme" />
          </FieldRow>
          <FieldRow label="Slug" for="gallery-slug" required hint="Lowercase letters and dashes. Fixed once saved.">
            <Input id="gallery-slug" value="acme" />
          </FieldRow>
          <FieldRow label="Contact" for="gallery-contact" error="Enter an email address.">
            <Input id="gallery-contact" value="not-an-email" aria-invalid="true" />
          </FieldRow>
          <FieldRow label="Internal note" for="gallery-note" stacked hint="Stacked stays stacked at every width, for narrow sheets.">
            <Input id="gallery-note" placeholder="Only the team sees this." />
          </FieldRow>
        </div>
      </div>
    </section>

    <section aria-labelledby="gal-detailsheet">
      <h2 id="gal-detailsheet" class="text-lg font-medium">DetailSheet</h2>
      <p class="text-muted-foreground mt-1 text-sm">One object in a full-height sheet with its own header, status and footer actions; a bottom sheet on phones. Opens from the buttons below.</p>
      <div class="mt-4 flex flex-wrap gap-2">
        <ActionButton kind="secondary" label="Open record…" icon="eye" @press="sheetOpen = true" />
        <ActionButton kind="secondary" label="Open wide…" icon="eye" @press="wideSheetOpen = true" />
      </div>
      <DetailSheet v-model:open="sheetOpen" title="Review the beta shell" description="Task · opened from the list.">
        <template #status>
          <StatusBadge status="open" />
        </template>
        <div class="space-y-4">
          <FieldRow label="Title" for="gallery-sheet-title" stacked>
            <Input id="gallery-sheet-title" value="Review the beta shell" />
          </FieldRow>
          <FieldRow label="Detail" for="gallery-sheet-detail" stacked>
            <Input id="gallery-sheet-detail" value="Check rail, bottom nav and palette." />
          </FieldRow>
        </div>
        <template #footer>
          <ActionGroup
            :actions="[
              { id: 'save', label: 'Save', kind: 'primary', icon: 'save' },
              { id: 'close', label: 'Close', kind: 'secondary', icon: 'x' },
            ]"
            stack-on-phones
            aria-label="Record actions"
            @press="(id) => { sheetOpen = false; if (id === 'save') toast.success('Saved (demo).') }"
          />
        </template>
      </DetailSheet>
      <DetailSheet v-model:open="wideSheetOpen" title="Ada Lovelace" description="Last sign-in today." wide>
        <template #status>
          <StatusBadge status="active" />
        </template>
        <p class="text-muted-foreground text-sm">The wide sheet fits detail lists: assignments, grants, versions.</p>
        <template #footer>
          <ActionGroup :actions="demoActions" stack-on-phones aria-label="User actions" @press="(id) => toast.info(`${id} (demo).`)" />
        </template>
      </DetailSheet>
    </section>

    <section aria-labelledby="gal-responsive">
      <h2 id="gal-responsive" class="text-lg font-medium">ResponsiveDialog</h2>
      <p class="text-muted-foreground mt-1 text-sm">A centered dialog at 1024 px and wider, a bottom drawer below. Type below, then dismiss: a dirty form asks first.</p>
      <div class="mt-4">
        <ActionButton kind="secondary" label="Open dialog…" icon="eye" @press="responsiveOpen = true" />
      </div>
      <ResponsiveDialog v-model:open="responsiveOpen" title="Demo dialog" description="Resize below 1024 px to see the drawer." :dirty="responsiveNote !== ''">
        <FieldRow label="Note" for="gallery-dialog-note" stacked>
          <Input id="gallery-dialog-note" v-model="responsiveNote" placeholder="Type to make it dirty" />
        </FieldRow>
        <template #footer>
          <ActionButton kind="secondary" label="Close" icon="x" @press="responsiveOpen = false" />
          <ActionButton kind="primary" label="Save" icon="save" @press="responsiveOpen = false; responsiveNote = ''; toast.success('Saved (demo).')" />
        </template>
      </ResponsiveDialog>
    </section>

    <section aria-labelledby="gal-statusbadge">
      <h2 id="gal-statusbadge" class="text-lg font-medium">StatusBadge</h2>
      <p class="text-muted-foreground mt-1 text-sm">One word of state. Unknown states render quiet.</p>
      <div class="mt-4 flex flex-wrap gap-2">
        <StatusBadge v-for="status in statuses" :key="status" :status="status" />
      </div>
    </section>

    <section aria-labelledby="gal-datatable">
      <h2 id="gal-datatable" class="text-lg font-medium">DataTable</h2>
      <p class="text-muted-foreground mt-1 text-sm">The one shared grid: toolbar with search, filters and Columns, select column with bulk actions, row numbers across pages, primary column with icon tile, row actions menu, and a footer with range, rows per page and Previous and Next. Below 640 px rows stack, filters move into a drawer, and selection is a long-press with a bottom action bar.</p>
      <div class="mt-4 space-y-4">
        <DataTable
          :columns="demoColumns"
          :data="demoFiltered"
          :get-row-id="(row) => row.id"
          v-model:selected-ids="demoSelected"
          :bulk-actions="[
            { id: 'export', label: 'Export', kind: 'secondary', icon: 'download' },
            { id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2' },
          ]"
          :row-actions="() => [
            { id: 'view', label: 'View', kind: 'quiet', icon: 'eye' },
            { id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2' },
          ]"
          v-model:search="demoSearch"
          search-placeholder="Find customers…"
          :filters="demoFilters"
          :loading="tableLoading"
          :error="tableError"
          :filtered="demoFilteredActive"
          clickable
          empty-title="No customers"
          empty-body="Invite one to see it here."
          @row-click="(row) => toast.info(`${row.name} (demo).`)"
          @row-action="demoRowAction"
          @bulk-action="demoBulk"
          @update:filters="demoFilters = $event"
          @retry="tableError = null; toast.info('Retry (demo).')"
        >
          <template #toolbar-actions>
            <ActionButton kind="primary" density="row" label="New customer" icon="plus" @press="toast.info('New customer (demo).')" />
          </template>
          <template #empty-action>
            <ActionButton kind="primary" density="row" label="Invite a customer" icon="plus" @press="demoRows = makeCustomers()" />
          </template>
        </DataTable>
        <div class="flex flex-wrap items-center gap-2">
          <ActionButton kind="quiet" density="row" label="Show empty state" icon="eye" @press="demoRows = []; demoSelected = []" />
          <ActionButton kind="quiet" density="row" label="Show filtered-empty state" icon="eye" @press="demoSearch = 'no such customer'" />
          <ActionButton kind="quiet" density="row" :label="tableLoading ? 'Hide loading' : 'Show loading'" icon="eye" @press="tableLoading = !tableLoading" />
          <ActionButton kind="quiet" density="row" label="Show error" icon="circle-alert" @press="tableError = 'The server took too long.'" />
          <ActionButton kind="restore" density="row" label="Restore rows" icon="rotate-ccw" @press="demoRows = makeCustomers(); demoSearch = ''; tableError = null" />
        </div>
      </div>
    </section>

    <section aria-labelledby="gal-datatable-server">
      <h2 id="gal-datatable-server" class="text-lg font-medium">DataTable, server paging</h2>
      <p class="text-muted-foreground mt-1 text-sm">Ninety payments through keyset pages of twenty-five, with no total: the footer shows the range and the page without a count. Selection would survive paging here too.</p>
      <div class="mt-4">
        <DataTable
          :columns="serverColumns"
          :data="serverSlice"
          :get-row-id="(row) => row.id"
          :server-paging="serverPaging"
          @prev-page="serverPage -= 1"
          @next-page="serverPage += 1"
          @update:page-size="(size) => { serverSize = size; serverPage = 1 }"
        />
      </div>
    </section>

    <section aria-labelledby="gal-builder">
      <h2 id="gal-builder" class="text-lg font-medium">ConditionBuilder</h2>
      <p class="text-muted-foreground mt-1 text-sm">No-code conditions: field, operator, value; groups join with match-all/any.</p>
      <div class="mt-4 max-w-3xl">
        <ConditionBuilder v-model="demoBuilderTree" :fields="demoBuilderFields" />
      </div>
    </section>

    <section aria-labelledby="gal-datepicker">
      <h2 id="gal-datepicker" class="text-lg font-medium">Date picker</h2>
      <p class="text-muted-foreground mt-1 text-sm">Calendar in a popover, per design-system section 7.</p>
      <div class="mt-4">
        <Popover>
          <PopoverTrigger as-child>
            <Button variant="outline" class="w-64 justify-start font-normal" aria-label="Pick a date">
              <CalendarDays class="size-4" aria-hidden="true" />
              <span>{{ pickedLabel }}</span>
            </Button>
          </PopoverTrigger>
          <PopoverContent class="w-auto p-0" align="start">
            <Calendar @update:model-value="onPick" />
          </PopoverContent>
        </Popover>
      </div>
    </section>
  </div>
</template>
