// The Record Types list (§19): a DataTable of every saved type on desktop
// and tablet, stacked rows with a summary drawer on phones. Row taps open
// the editor, except on phones where they open the summary drawer first.
<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import ActionButton from '@/components/ActionButton.vue'
import DataTable, { type TableFilter } from '@/components/DataTable.vue'
import PageHeader from '@/components/PageHeader.vue'
import PullToRefresh from '@/components/PullToRefresh.vue'
import RecordTypeSummaryDrawer from '@/components/record-types/RecordTypeSummaryDrawer.vue'
import { filterRecordTypes, kindLabel, kindOptions, listSortValues, packLabel } from '@/components/record-types/recordTypeList'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { recordTypeIcon as fallbackIcon } from '@/pages/registry'
import { formatWhen, formatWhenFull } from '@/times'
import type { RecordType } from '@/types'

const props = withDefaults(defineProps<{ recordTypes: RecordType[]; typesLoaded: boolean; typesLoadFailed: boolean; username?: string }>(), { username: '' })
const emit = defineEmits<{
  createRecordType: []
  openEditor: [key: string]
  viewRecords: [key: string]
  reloadRecordTypes: []
}>()

const search = ref('')
const kindSelected = ref<string[]>([])
const packSelected = ref<string[]>([])
const drawerRow = ref<RecordType | null>(null)
const drawerOpen = ref(false)
const isPhone = useMediaQuery('(max-width: 639px)')
const refreshing = ref(false)
watch(() => props.recordTypes, () => { refreshing.value = false })

const filters = computed<TableFilter[]>(() => {
  const packs = [...new Set(props.recordTypes.map((type) => type.pack_key).filter((pack): pack is string => !!pack))].sort()
  return [
    {
      id: 'kind', label: 'Kind', hint: 'recordKind',
      options: kindOptions.filter((option) => option.value !== 'all').map((option) => ({ value: option.value, label: option.label })),
      selected: kindSelected.value,
    },
    {
      id: 'pack', label: 'Pack', hint: 'pack',
      options: [{ value: 'none', label: 'None' }, ...packs.map((pack) => ({ value: pack, label: pack }))],
      selected: packSelected.value,
    },
  ]
})
const filtered = computed(() => filterRecordTypes(props.recordTypes, search.value, kindSelected.value, packSelected.value))
const hasActiveFilters = computed(() => search.value.trim() !== '' || kindSelected.value.length > 0 || packSelected.value.length > 0)

function onFiltersChanged(next: TableFilter[]) {
  kindSelected.value = next.find((filter) => filter.id === 'kind')?.selected ?? []
  packSelected.value = next.find((filter) => filter.id === 'pack')?.selected ?? []
}

function onRowClick(row: RecordType) {
  if (isPhone.value) {
    drawerRow.value = row
    drawerOpen.value = true
  } else {
    emit('openEditor', row.name)
  }
}

function onRefresh() {
  refreshing.value = true
  emit('reloadRecordTypes')
}

function muted(text: string) {
  return h('span', { class: 'text-muted-foreground' }, text)
}

const columns = [
  {
    id: 'name', header: 'Name', accessorFn: (type: RecordType) => listSortValues.name(type), meta: { label: 'Name' },
    cell: ({ row }: { row: { original: RecordType } }) => h('span', { class: 'flex items-center gap-2' }, [h(fallbackIcon, { class: 'text-muted-foreground size-4 shrink-0', 'aria-hidden': true }), h('span', { class: 'font-medium' }, row.original.label || 'Untitled')]),
  },
  {
    id: 'description', header: 'Description', accessorFn: (type: RecordType) => listSortValues.description(type), meta: { label: 'Description', hideOnMobile: true },
    cell: ({ row }: { row: { original: RecordType } }) => muted(row.original.description),
  },
  {
    id: 'kind', header: 'Kind', accessorFn: (type: RecordType) => listSortValues.kind(type), meta: { label: 'Kind' },
    cell: ({ row }: { row: { original: RecordType } }) => h(Badge, { variant: 'secondary' }, () => kindLabel(row.original.kind || 'master')),
  },
  {
    id: 'fields', header: 'Fields', accessorFn: (type: RecordType) => listSortValues.fields(type), meta: { label: 'Fields', numeric: true },
    cell: ({ row }: { row: { original: RecordType } }) => h('span', { class: 'tabular-nums' }, String(row.original.fields.length)),
  },
  {
    id: 'records', header: 'Records', accessorFn: (type: RecordType) => listSortValues.records(type), meta: { label: 'Records', numeric: true, hideOnMobile: true },
    cell: ({ row }: { row: { original: RecordType } }) => {
      if (!row.original.count_hidden) return h('span', { class: 'tabular-nums' }, String(row.original.record_count ?? 0))
      return h(Tooltip, null, {
        default: () => h(TooltipTrigger, { asChild: true }, () => muted('Hidden')),
        content: () => h(TooltipContent, null, () => 'You cannot read these records, so the count stays hidden.'),
      })
    },
  },
  {
    id: 'pack', header: 'Pack', accessorFn: (type: RecordType) => listSortValues.pack(type), meta: { label: 'Pack', hideOnMobile: true },
    cell: ({ row }: { row: { original: RecordType } }) => (row.original.pack_key ? row.original.pack_key : muted(packLabel(row.original.pack_key))),
  },
  {
    id: 'updated', header: 'Last changed', accessorFn: (type: RecordType) => listSortValues.updated(type), meta: { label: 'Last changed', hideOnMobile: true },
    cell: ({ row }: { row: { original: RecordType } }) => {
      if (!row.original.updated_at) return muted('—')
      return h(Tooltip, null, {
        default: () => h(TooltipTrigger, { asChild: true }, () => h('span', null, formatWhen(row.original.updated_at ?? ''))),
        content: () => h(TooltipContent, null, () => formatWhenFull(row.original.updated_at ?? '')),
      })
    },
  },
  {
    id: 'changedBy', header: 'Changed by', accessorFn: (type: RecordType) => listSortValues.changedBy(type), meta: { label: 'Changed by', hideOnMobile: true },
    cell: ({ row }: { row: { original: RecordType } }) => (row.original.updated_by ? row.original.updated_by : muted('—')),
  },
]
</script>

<template>
  <section class="page-content builder-page">
    <PageHeader title="Record Types" description="Each Record Type defines the fields of one kind of record.">
      <template #actions>
        <ActionButton kind="primary" label="New Record Type" icon="plus" @press="emit('createRecordType')" />
      </template>
    </PageHeader>
    <PullToRefresh v-model:refreshing="refreshing" @refresh="onRefresh">
      <TooltipProvider :delay-duration="500">
        <DataTable
          class="mt-4"
          :columns="columns"
          :data="filtered"
          table-id="record-types"
          :username="username"
          :get-row-id="(type) => type.name"
          :row-actions="() => [{ id: 'view', label: 'View records', kind: 'quiet', icon: 'eye' }]"
          v-model:search="search"
          search-placeholder="Find a Record Type"
          :filters="filters"
          :loading="!typesLoaded && !typesLoadFailed"
          :error="typesLoadFailed ? 'The list did not load. Try again.' : null"
          :filtered="hasActiveFilters"
          clickable
          :empty-icon="fallbackIcon"
          empty-title="No Record Types yet"
          empty-body="Create a Record Type, add its fields and save it to start adding records."
          empty-art="empty-record-types"
          filtered-art="empty-sieve"
          @row-click="onRowClick"
          @row-action="(id, type) => emit('viewRecords', type.name)"
          @update:filters="onFiltersChanged"
          @retry="emit('reloadRecordTypes')"
        >
          <template #empty-action>
            <ActionButton kind="primary" label="Create your first Record Type" icon="plus" @press="emit('createRecordType')" />
          </template>
        </DataTable>
      </TooltipProvider>
    </PullToRefresh>
    <RecordTypeSummaryDrawer
      v-model:open="drawerOpen"
      :record-type="drawerRow"
      @open-editor="emit('openEditor', $event)"
      @view-records="emit('viewRecords', $event)"
    />
  </section>
</template>

