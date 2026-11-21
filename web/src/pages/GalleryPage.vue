// Gallery shows every shared composition in every state. It is a Build page
// for designers and developers; it stays out of the nav and loads by address
// (?mode=build&page=gallery, /build/gallery).
<script setup lang="ts">
import { computed, ref } from 'vue'
import { CalendarDays, ChartColumn, Inbox } from '@/icons'

import { toast } from 'vue-sonner'
import ActionButton from '@/components/ActionButton.vue'
import ActionGroup from '@/components/ActionGroup.vue'
import type { ActionItem } from '@/components/actions'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import DataTable from '@/components/DataTable.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import FieldRow from '@/components/FieldRow.vue'
import HelpHint from '@/copy/HelpHint.vue'
import FormSection from '@/components/FormSection.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import PageHeader from '@/components/PageHeader.vue'
import PermissionDenied from '@/components/PermissionDenied.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

const emit = defineEmits<{ goHome: [] }>()

const dangerOpen = ref(false)
const plainOpen = ref(false)
const busyOpen = ref(false)
const busyWorking = ref(false)
const disabledOpen = ref(false)
const sheetOpen = ref(false)
const wideSheetOpen = ref(false)
const galleryName = ref('')
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

type DemoRow = { name: string; kind: string; records: number }
const demoColumns = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'kind', header: 'Kind', meta: { label: 'Kind', hideOnMobile: true } },
  { accessorKey: 'records', header: 'Records' },
]
const demoRows = ref<DemoRow[]>([
  { name: 'Task', kind: 'master', records: 12 },
  { name: 'Note', kind: 'master', records: 3 },
  { name: 'Invoice', kind: 'document', records: 48 },
])
const tableEmpty = computed(() => demoRows.value.length === 0)
const tableLoading = ref(false)
const tableError = ref<string | null>(null)

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
      <p class="text-muted-foreground mt-1 text-sm">Nothing here yet: icon, title, one helpful line, optional action.</p>
      <div class="mt-4 grid grid-cols-1 gap-4 min-[640px]:grid-cols-2">
        <div class="rounded-[var(--radius)] border">
          <EmptyState :icon="Inbox" title="All caught up" body="Records assigned to you will show up here." />
        </div>
        <div class="rounded-[var(--radius)] border">
          <EmptyState :icon="ChartColumn" title="No reports yet" body="Build a report to rerun a list or total any time.">
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
      <p class="text-muted-foreground mt-1 text-sm">Sortable grid; rows stack into labelled cards below 640 px, where sorting opens from a toolbar drawer. Kind hides on phones. Click a row.</p>
      <div class="mt-4 space-y-4">
        <DataTable
          :columns="demoColumns"
          :data="demoRows"
          clickable
          :loading="tableLoading"
          :error="tableError"
          empty-title="No record types"
          empty-body="Define one to see it here."
          @row-click="(row) => toast.info(`${row.name} (demo).`)"
          @retry="tableError = null; toast.info('Retry (demo).')"
        />
        <div class="flex flex-wrap items-center gap-2">
          <ActionButton kind="quiet" density="row" label="Show empty state" icon="eye" @press="demoRows = []" />
          <ActionButton v-if="tableEmpty" kind="restore" density="row" label="Restore one row" icon="rotate-ccw" @press="demoRows = [{ name: 'Task', kind: 'master', records: 12 }]" />
          <ActionButton kind="quiet" density="row" :label="tableLoading ? 'Hide loading' : 'Show loading'" icon="eye" @press="tableLoading = !tableLoading" />
          <ActionButton kind="quiet" density="row" label="Show error" icon="circle-alert" @press="tableError = 'The server took too long.'" />
        </div>
        <DataTable v-if="tableEmpty && !tableLoading && !tableError" :columns="demoColumns" :data="demoRows" :empty-icon="Inbox" empty-title="No record types" empty-body="Define one to see it here." />
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
