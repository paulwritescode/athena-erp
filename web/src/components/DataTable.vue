// DataTable is the shared grid: TanStack sorting, an empty state, and rows
// that stack into labelled cards below 640 px. Columns are TanStack column
// definitions; a column sets meta.label to name its stacked row (else the
// string header is used) and meta.hideOnMobile to drop out of the stacked
// rows. On phones the header hides, so a sort select sits above the rows.
<script setup lang="ts" generic="TData extends RowData">
import { computed, ref } from 'vue'
import {
  createSortedRowModel,
  FlexRender,
  rowSortingFeature,
  sortFn_alphanumeric,
  tableFeatures,
  useTable,
  type ColumnDef,
  type RowData,
  type SortingState,
  type Updater,
} from '@tanstack/vue-table'
import { ArrowDown, ArrowUp, ArrowUpDown, Inbox } from '@lucide/vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Component } from 'vue'

const features = tableFeatures({ rowSortingFeature, sortedRowModel: createSortedRowModel(), sortFns: { alphanumeric: sortFn_alphanumeric } })
type DataColumn<T extends RowData> = ColumnDef<typeof features, T, any>
type ColumnMeta = { label?: string; hideOnMobile?: boolean }

const props = withDefaults(defineProps<{
  columns: DataColumn<TData>[]
  data: TData[]
  emptyIcon?: Component
  emptyTitle?: string
  emptyBody?: string
  clickable?: boolean
  loading?: boolean
  error?: string | null
}>(), {
  emptyIcon: () => Inbox,
  emptyTitle: 'Nothing here yet',
  emptyBody: 'Rows will appear here when there is something to show.',
  clickable: false,
  loading: false,
  error: null,
})
const emit = defineEmits<{ rowClick: [row: TData]; retry: [] }>()

const sorting = ref<SortingState>([])
const state = computed(() => ({ sorting: sorting.value }))
const table = useTable({
  features,
  get columns() { return props.columns },
  get data() { return props.data },
  state,
  onSortingChange: (next: Updater<SortingState>) => {
    sorting.value = typeof next === 'function' ? next(sorting.value) : next
  },
})

function stackLabel(column: { columnDef: { meta?: unknown; header?: unknown } }): string {
  const meta = column.columnDef.meta as ColumnMeta | undefined
  if (meta?.label) return meta.label
  return typeof column.columnDef.header === 'string' ? column.columnDef.header : ''
}

function hiddenOnMobile(column: { columnDef: { meta?: unknown } }): boolean {
  return (column.columnDef.meta as ColumnMeta | undefined)?.hideOnMobile === true
}

const sortableColumns = computed(() => table.getAllColumns().filter((column) => column.getCanSort()))
const sortId = computed(() => sorting.value[0]?.id ?? '')
const sortDesc = computed(() => sorting.value[0]?.desc === true)

function chooseSort(id: string) {
  if (!id) {
    table.resetSorting()
    return
  }
  table.getColumn(id)?.toggleSorting(false)
}

function flipDirection() {
  const current = sorting.value[0]
  if (current) table.getColumn(current.id)?.toggleSorting(!current.desc)
}

function activate(row: TData) {
  if (props.clickable) emit('rowClick', row)
}
</script>

<template>
  <div>
    <div v-if="sortableColumns.length > 0" class="mb-3 flex items-center gap-2 min-[640px]:hidden">
      <NativeSelect class="min-w-0 flex-1" :model-value="sortId" aria-label="Sort rows" @update:model-value="chooseSort(String($event))">
        <NativeSelectOption value="">Sort…</NativeSelectOption>
        <NativeSelectOption v-for="column in sortableColumns" :key="column.id" :value="column.id">
          {{ stackLabel(column) || column.id }}
        </NativeSelectOption>
      </NativeSelect>
      <Button variant="outline" size="icon" :disabled="!sortId" :aria-label="sortDesc ? 'Sort ascending' : 'Sort descending'" @click="flipDirection">
        <component :is="sortDesc ? ArrowDown : ArrowUp" class="size-4" aria-hidden="true" />
      </Button>
    </div>
    <div class="datatable-stacked overflow-x-auto rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow v-for="headerGroup in table.getHeaderGroups()" :key="headerGroup.id">
            <TableHead v-for="header in headerGroup.headers" :key="header.id" :colspan="header.colSpan">
              <Button
                v-if="!header.isPlaceholder && header.column.getCanSort()"
                variant="ghost"
                size="sm"
                class="-ml-2"
                :aria-label="`Sort by ${stackLabel(header.column) || 'this column'}`"
                @click="header.column.getToggleSortingHandler()?.($event)"
              >
                <FlexRender :header="header" />
                <ArrowUpDown class="size-3.5" aria-hidden="true" />
              </Button>
              <FlexRender v-else-if="!header.isPlaceholder" :header="header" />
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <template v-if="loading">
            <TableRow v-for="row in 3" :key="row">
              <TableCell v-for="column in table.getAllColumns()" :key="column.id">
                <Skeleton class="h-6 w-full" />
              </TableCell>
            </TableRow>
          </template>
          <TableEmpty v-else-if="error" :colspan="props.columns.length">
            <ErrorState title="These rows did not load" :message="error ?? 'Try again.'" @retry="emit('retry')" />
          </TableEmpty>
          <template v-else-if="table.getRowModel().rows.length > 0">
            <TableRow
              v-for="row in table.getRowModel().rows"
              :key="row.id"
              :tabindex="clickable ? 0 : undefined"
              :class="clickable ? 'cursor-pointer focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none' : undefined"
              @click="activate(row.original as TData)"
              @keydown.enter="activate(row.original as TData)"
              @keydown.space.prevent="activate(row.original as TData)"
            >
              <TableCell
                v-for="cell in row.getAllCells()"
                :key="cell.id"
                :data-label="stackLabel(cell.column)"
                :class="hiddenOnMobile(cell.column) ? 'datatable-hide-mobile' : undefined"
              >
                <FlexRender :cell="cell" />
              </TableCell>
            </TableRow>
          </template>
          <TableEmpty v-else :colspan="props.columns.length">
            <EmptyState :icon="emptyIcon" :title="emptyTitle" :body="emptyBody" />
          </TableEmpty>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
