// The Record Types list (§19): a DataTable of every saved type on desktop
// and tablet, stacked rows with a summary drawer on phones. Row taps open
// the editor, except on phones where they open the summary drawer first.
<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import ActionButton from '@/components/ActionButton.vue'
import HelpHint from '@/copy/HelpHint.vue'
import BottomDrawer from '@/components/BottomDrawer.vue'
import DataTable from '@/components/DataTable.vue'
import EmptyState from '@/components/EmptyState.vue'
import PageHeader from '@/components/PageHeader.vue'
import PullToRefresh from '@/components/PullToRefresh.vue'
import RecordTypeSummaryDrawer from '@/components/record-types/RecordTypeSummaryDrawer.vue'
import { filterRecordTypes, kindLabel, kindOptions, listSortValues, packLabel } from '@/components/record-types/recordTypeList'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { recordTypeIcon as fallbackIcon } from '@/pages/registry'
import { formatWhen, formatWhenFull } from '@/times'
import type { RecordType } from '@/types'

const props = defineProps<{ recordTypes: RecordType[]; typesLoaded: boolean; typesLoadFailed: boolean }>()
const emit = defineEmits<{
  createRecordType: []
  openEditor: [key: string]
  viewRecords: [key: string]
  reloadRecordTypes: []
}>()

const search = ref('')
const kindFilter = ref('all')
const packFilter = ref('all')
const filtersOpen = ref(false)
const drawerRow = ref<RecordType | null>(null)
const drawerOpen = ref(false)
const isPhone = useMediaQuery('(max-width: 639px)')
const refreshing = ref(false)
watch(() => props.recordTypes, () => { refreshing.value = false })

const packOptions = computed(() => {
  const packs = [...new Set(props.recordTypes.map((type) => type.pack_key).filter((pack): pack is string => !!pack))].sort()
  return [
    { value: 'all', label: 'All packs' },
    { value: 'none', label: 'None' },
    ...packs.map((pack) => ({ value: pack, label: pack })),
  ]
})
const filtered = computed(() => filterRecordTypes(props.recordTypes, search.value, kindFilter.value, packFilter.value))
const hasActiveFilters = computed(() => search.value.trim() !== '' || kindFilter.value !== 'all' || packFilter.value !== 'all')

function clearFilters() {
  search.value = ''
  kindFilter.value = 'all'
  packFilter.value = 'all'
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
    id: 'fields', header: 'Fields', accessorFn: (type: RecordType) => listSortValues.fields(type), meta: { label: 'Fields' },
    cell: ({ row }: { row: { original: RecordType } }) => h('span', { class: 'tabular-nums' }, String(row.original.fields.length)),
  },
  {
    id: 'records', header: 'Records', accessorFn: (type: RecordType) => listSortValues.records(type), meta: { label: 'Records', hideOnMobile: true },
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
    <EmptyState
      v-if="typesLoaded && !typesLoadFailed && recordTypes.length === 0"
      :icon="fallbackIcon"
      title="No Record Types yet"
      body="Create a Record Type, add its fields and save it to start adding records."
    >
      <template #action>
        <ActionButton kind="primary" label="Create your first Record Type" icon="plus" @press="emit('createRecordType')" />
      </template>
    </EmptyState>
    <PullToRefresh v-else v-model:refreshing="refreshing" @refresh="onRefresh">
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <Input v-model="search" class="min-w-40 flex-1" placeholder="Find a Record Type" aria-label="Find a Record Type" />
        <div class="hidden min-[640px]:contents">
          <NativeSelect v-model="kindFilter" class="w-36" aria-label="Kind filter">
            <NativeSelectOption v-for="option in kindOptions" :key="option.value" :value="option.value">{{ option.label }}</NativeSelectOption>
          </NativeSelect>
          <NativeSelect v-model="packFilter" class="w-36" aria-label="Pack filter">
            <NativeSelectOption v-for="option in packOptions" :key="option.value" :value="option.value">{{ option.label }}</NativeSelectOption>
          </NativeSelect>
          <Button v-if="hasActiveFilters" variant="link" @click="clearFilters">Clear all</Button>
        </div>
        <ActionButton class="min-[640px]:hidden" kind="secondary" label="Filters" icon="list-filter" @press="filtersOpen = true" />
      </div>
      <BottomDrawer v-model:open="filtersOpen" title="Filters">
        <div class="space-y-4">
          <label class="block text-sm font-medium">Kind <HelpHint term="recordKind" />
            <NativeSelect v-model="kindFilter" class="mt-1 w-full" aria-label="Kind filter">
              <NativeSelectOption v-for="option in kindOptions" :key="option.value" :value="option.value">{{ option.label }}</NativeSelectOption>
            </NativeSelect>
          </label>
          <label class="block text-sm font-medium">Pack <HelpHint term="pack" />
            <NativeSelect v-model="packFilter" class="mt-1 w-full" aria-label="Pack filter">
              <NativeSelectOption v-for="option in packOptions" :key="option.value" :value="option.value">{{ option.label }}</NativeSelectOption>
            </NativeSelect>
          </label>
          <Button v-if="hasActiveFilters" variant="link" class="px-0" @click="clearFilters">Clear all</Button>
        </div>
        <template #footer>
          <ActionButton kind="primary" label="Done" icon="check" full-bleed @press="filtersOpen = false" />
        </template>
      </BottomDrawer>
      <TooltipProvider :delay-duration="500">
        <DataTable
          class="mt-4"
          :columns="columns"
          :data="filtered"
          :loading="!typesLoaded && !typesLoadFailed"
          :error="typesLoadFailed ? 'The list did not load. Try again.' : null"
          clickable
          empty-title="No matching Record Types"
          empty-body="Try a different search, or clear the filters."
          @row-click="onRowClick"
          @retry="emit('reloadRecordTypes')"
        />
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

