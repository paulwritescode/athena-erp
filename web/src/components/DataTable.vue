// DataTable is the one shared grid (§21): a bordered full-width frame with
// padding, a select column with bulk actions, row numbers across pages, a
// toolbar (search, filter popovers with value chips, Clear filters, Columns
// saved per user), a selection bar, and a footer (range, rows per page saved
// per user, Page x of y, Previous and Next). Client tables page in memory;
// server tables pass serverPaging and answer prevPage/nextPage with keyset
// tokens. Rows stack into labelled cards below 640 px, where filters,
// sorting and columns move into one drawer and selection is a long-press
// with a bottom action bar. Column meta: label (stacked row name),
// hideOnMobile (drops from stacked rows), numeric (right-aligned tabular),
// center, hideable (in the Columns list, default true).
<script setup lang="ts" generic="TData extends RowData">
import { computed, onUnmounted, ref, useSlots, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import {
  FlexRender,
  useTable,
  type ColumnSizingState,
  type RowData,
  type SortingState,
  type Updater,
} from '@tanstack/vue-table'
import { ArrowDown, ArrowUp, ArrowUpDown, Check, ChevronDown, Ellipsis, Inbox, Minus, Plus, Search, X, icon } from '@/icons'
import ActionButton from '@/components/ActionButton.vue'
import ActionGroup from '@/components/ActionGroup.vue'
import BottomDrawer from '@/components/BottomDrawer.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { chunkByKind, type ActionItem } from './actions'
import { dataTableFeatures, type DataTableColumn } from './dataTableFeatures'
import HelpHint from '@/copy/HelpHint.vue'
import type { GlossaryKey } from '@/copy/glossary'
import { pageText, rangeText, rowNumber, selectionText } from '@/dataTableModel'
import { PAGE_SIZE_OPTIONS, loadHiddenColumns, loadPageSize, saveHiddenColumns, savePageSize } from '@/tablePrefs'
import type { Component } from 'vue'

export type TableFilterOption = { value: string; label: string }
export type TableFilter = { id: string; label: string; options: TableFilterOption[]; selected: string[]; hint?: GlossaryKey }
export type ServerPaging = { page: number; total?: number | null; hasNext: boolean }
export type DataTableColumnMeta = { label?: string; hideOnMobile?: boolean; numeric?: boolean; center?: boolean; hideable?: boolean }

const features = dataTableFeatures
type DataColumn<T extends RowData> = DataTableColumn<T>

const props = withDefaults(defineProps<{
  columns: DataColumn<TData>[]
  data: TData[]
  emptyIcon?: Component
  emptyTitle?: string
  emptyBody?: string
  /** Art registry keys for the unfiltered and filtered empties. At most one
      art-carrying table shows per screen (§9.2): secondary tables on a page
      leave both unset and keep the icon empty. */
  emptyArt?: string
  filteredArt?: string
  clickable?: boolean
  loading?: boolean
  error?: string | null
  tableId?: string
  username?: string
  getRowId?: (row: TData) => string
  selectedIds?: string[]
  bulkActions?: ActionItem[]
  rowActions?: (row: TData) => ActionItem[]
  search?: string
  searchPlaceholder?: string
  filters?: TableFilter[]
  serverPaging?: ServerPaging | null
  showRowNumbers?: boolean
  resizable?: boolean
  widthKey?: string
  filtered?: boolean
}>(), {
  emptyIcon: () => Inbox,
  emptyTitle: 'Nothing here yet',
  emptyBody: 'Rows will appear here when there is something to show.',
  clickable: false,
  loading: false,
  error: null,
  tableId: '',
  username: '',
  selectedIds: () => [],
  bulkActions: () => [],
  rowActions: undefined,
  getRowId: undefined,
  search: '',
  searchPlaceholder: '',
  filters: () => [],
  serverPaging: null,
  showRowNumbers: true,
  resizable: false,
  widthKey: '',
  filtered: false,
})
const emit = defineEmits<{
  rowClick: [row: TData]
  retry: []
  'update:selectedIds': [ids: string[]]
  'update:search': [value: string]
  'update:filters': [filters: TableFilter[]]
  clearFilters: []
  'update:pageSize': [size: number]
  prevPage: []
  nextPage: []
  bulkAction: [id: string]
  rowAction: [id: string, row: TData]
}>()
const slots = useSlots()
const serverMode = computed(() => props.serverPaging !== null)
const hasBulk = computed(() => props.bulkActions.length > 0)
const hasRowActions = computed(() => props.rowActions !== undefined)
const isPhone = useMediaQuery('(max-width: 639px)')

function rowKey(row: TData, index: number): string {
  if (props.getRowId) return props.getRowId(row)
  const id = (row as Record<string, unknown>).id
  return typeof id === 'string' && id ? id : String(index)
}

// Selection is a set of row ids owned here, mirrored to v-model. Ids
// survive paging and sorting; the caller clears them after a bulk action.
const selected = ref(new Set<string>(props.selectedIds))
watch(() => props.selectedIds, (ids) => { selected.value = new Set(ids) })
function setSelected(next: Set<string>) {
  selected.value = next
  emit('update:selectedIds', [...next])
}
function isSelected(id: string): boolean {
  return selected.value.has(id)
}
function toggleRow(id: string) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  setSelected(next)
}
function clearSelection() {
  setSelected(new Set())
}

// Column visibility and page size persist per user when the table has an
// id; without one they live for the session only (the Gallery demo).
const hiddenIds = ref(loadHiddenColumns(props.tableId, props.username))
watch([() => props.tableId, () => props.username], ([id, user]) => {
  hiddenIds.value = loadHiddenColumns(id, user)
  pageSize.value = loadPageSize(id, user)
})
const pageSize = ref(loadPageSize(props.tableId, props.username))
const clientPage = ref(1)

function loadSizing(): ColumnSizingState {
  if (!props.widthKey || typeof globalThis.localStorage === 'undefined') return {}
  try {
    const parsed: unknown = JSON.parse(globalThis.localStorage.getItem(props.widthKey) ?? '{}')
    if (!parsed || typeof parsed !== 'object') return {}
    return Object.fromEntries(Object.entries(parsed).filter((pair): pair is [string, number] => typeof pair[1] === 'number' && Number.isFinite(pair[1])))
  } catch {
    return {}
  }
}
const columnSizing = ref<ColumnSizingState>(loadSizing())
watch(columnSizing, (value) => {
  if (props.resizable && props.widthKey && typeof globalThis.localStorage !== 'undefined') {
    try {
      globalThis.localStorage.setItem(props.widthKey, JSON.stringify(value))
    } catch {
      // Private browsing must never break the table.
    }
  }
}, { deep: true })

const sorting = ref<SortingState>([])
const table = useTable({
  features,
  get columns() { return props.columns.filter((column) => !hiddenIds.value.has(columnId(column))) },
  get data() { return props.data },
  getRowId: (row: TData, index: number) => rowKey(row, index),
  columnResizeMode: 'onChange',
  state: computed(() => ({ sorting: serverMode.value ? [] : sorting.value, columnSizing: columnSizing.value })),
  onSortingChange: (next: Updater<SortingState>) => {
    sorting.value = typeof next === 'function' ? next(sorting.value) : next
  },
  onColumnSizingChange: (next: Updater<ColumnSizingState>) => {
    columnSizing.value = typeof next === 'function' ? next(columnSizing.value) : next
  },
})

function columnId(column: DataColumn<TData>): string {
  if (typeof column.id === 'string' && column.id) return column.id
  const key = (column as { accessorKey?: unknown }).accessorKey
  return typeof key === 'string' && key ? key : ''
}

function columnMeta(column: { columnDef: { meta?: unknown } }): DataTableColumnMeta {
  return (column.columnDef.meta as DataTableColumnMeta | undefined) ?? {}
}

function stackLabel(column: { columnDef: { meta?: unknown; header?: unknown } }): string {
  const meta = columnMeta(column)
  if (meta.label) return meta.label
  return typeof column.columnDef.header === 'string' ? column.columnDef.header : ''
}

function hiddenOnMobile(column: { columnDef: { meta?: unknown } }): boolean {
  return columnMeta(column).hideOnMobile === true
}

function numericColumn(column: { columnDef: { meta?: unknown } }): boolean {
  return columnMeta(column).numeric === true
}

function centerColumn(column: { columnDef: { meta?: unknown } }): boolean {
  return columnMeta(column).center === true
}

// In server mode the page arrives sorted from the API (RecordService takes
// no sort parameters), so sorting stays off rather than sorting one page.
const sortableColumns = computed(() => serverMode.value ? [] : table.getAllColumns().filter((column) => column.getCanSort()))

function toggleColumn(id: string) {
  const next = new Set(hiddenIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    if (table.getAllColumns().length <= 1) return
    next.add(id)
  }
  hiddenIds.value = next
  saveHiddenColumns(props.tableId, props.username, next)
}
// The Columns list reads every definition, including hidden ones, so a
// hidden column can always be shown again.
const toggleColumns = computed(() => props.columns
  .filter((column) => ((column.meta as DataTableColumnMeta | undefined)?.hideable !== false))
  .map((column) => {
    const id = columnId(column)
    const meta = (column.meta as DataTableColumnMeta | undefined) ?? {}
    return { id, label: meta.label ?? (typeof column.header === 'string' ? column.header : id), visible: !hiddenIds.value.has(id) }
  })
  .filter((column) => column.id !== ''))
const showColumnsButton = computed(() => toggleColumns.value.length > 0)
const visibleDataCount = computed(() => table.getAllColumns().length)

// Paging: client tables slice the sorted rows; server tables show the page
// the API returned. The total is null on keyset pages without one.
const sortedRows = computed(() => table.getRowModel().rows)
const total = computed<number | null>(() => serverMode.value ? (props.serverPaging?.total ?? null) : props.data.length)
const page = computed(() => serverMode.value ? (props.serverPaging?.page ?? 1) : clientPage.value)
const pageRows = computed(() => serverMode.value
  ? sortedRows.value
  : sortedRows.value.slice((clientPage.value - 1) * pageSize.value, clientPage.value * pageSize.value))
const hasNext = computed(() => serverMode.value
  ? (props.serverPaging?.hasNext ?? false)
  : clientPage.value * pageSize.value < sortedRows.value.length)
const hasPrev = computed(() => page.value > 1)
const rangeInput = computed(() => ({ pageRowCount: pageRows.value.length, page: page.value, pageSize: pageSize.value, total: total.value, hasNext: hasNext.value }))
const rangeLabel = computed(() => rangeText(rangeInput.value))
const pageLabel = computed(() => pageText(rangeInput.value))
const selectionLabel = computed(() => selectionText(selected.value.size, total.value))

watch(() => sortedRows.value.length, () => {
  if (serverMode.value) return
  const last = Math.max(1, Math.ceil(sortedRows.value.length / pageSize.value))
  if (clientPage.value > last) clientPage.value = last
})
// A new search or filter restarts a client table on page one.
watch([() => props.search, () => props.filters], () => {
  if (!serverMode.value) clientPage.value = 1
}, { deep: true })

function changePageSize(size: number) {
  pageSize.value = size
  savePageSize(props.tableId, props.username, size)
  if (!serverMode.value) clientPage.value = 1
  emit('update:pageSize', size)
}

function prev() {
  if (!hasPrev.value) return
  if (serverMode.value) emit('prevPage')
  else clientPage.value -= 1
}

function next() {
  if (!hasNext.value) return
  if (serverMode.value) emit('nextPage')
  else clientPage.value += 1
}

// Selection across the page: the header box ticks the page, with an
// indeterminate state, and never touches other pages.
const pageIds = computed(() => pageRows.value.map((row) => row.id))
const allPageSelected = computed(() => pageIds.value.length > 0 && pageIds.value.every((id) => selected.value.has(id)))
const somePageSelected = computed(() => pageIds.value.some((id) => selected.value.has(id)))
function togglePage() {
  const next = new Set(selected.value)
  if (allPageSelected.value) for (const id of pageIds.value) next.delete(id)
  else for (const id of pageIds.value) next.add(id)
  setSelected(next)
}

// Toolbar models. Search and filters belong to the caller (they shape the
// query); the table only edits and clears them.
const searchDraft = computed({
  get: () => props.search,
  set: (value: string) => emit('update:search', value),
})
const showSearch = computed(() => props.searchPlaceholder !== '')
const hasActiveFilters = computed(() => searchDraft.value.trim() !== '' || props.filters.some((filter) => filter.selected.length > 0))
const showSelectionBar = computed(() => hasBulk.value && selected.value.size > 0)
const showToolbar = computed(() => showSearch.value || props.filters.length > 0 || showColumnsButton.value || hasBulk.value || !!slots['toolbar-actions'])

function filterSummary(filter: TableFilter): string {
  const labels = filter.selected.map((value) => filter.options.find((option) => option.value === value)?.label ?? value)
  if (labels.length === 0) return filter.label
  if (labels.length === 1) return `${filter.label}: ${labels[0]}`
  return `${filter.label}: ${labels[0]} +${labels.length - 1}`
}

function setFilterSelected(id: string, values: string[]) {
  emit('update:filters', props.filters.map((filter) => filter.id === id ? { ...filter, selected: values } : filter))
}

function toggleFilterValue(filter: TableFilter, value: string) {
  const values = filter.selected.includes(value)
    ? filter.selected.filter((item) => item !== value)
    : [...filter.selected, value]
  setFilterSelected(filter.id, values)
}

function clearAllFilters() {
  emit('update:search', '')
  emit('update:filters', props.filters.map((filter) => ({ ...filter, selected: [] })))
  emit('clearFilters')
}

// Mobile sorting, filters and columns share one Filters drawer.
const filtersOpen = ref(false)
const sortId = computed(() => sorting.value[0]?.id ?? '')
const sortDesc = computed(() => sorting.value[0]?.desc === true)
const showFiltersButton = computed(() => sortableColumns.value.length > 0 || props.filters.length > 0 || showColumnsButton.value)

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

// Phone selection: a long-press opens select mode and ticks the row;
// leaving the last row unticked closes it again. The release that follows
// a long-press also fires a click, which would untick the row at once, so
// clicks stay suppressed briefly after a press lands.
const selectMode = ref(false)
let pressTimer: ReturnType<typeof setTimeout> | undefined
let suppressClickUntil = 0
watch(selected, (value) => { if (value.size === 0) selectMode.value = false })
onUnmounted(() => clearTimeout(pressTimer))

function onTouchStart(id: string) {
  if (!isPhone.value || !hasBulk.value) return
  clearTimeout(pressTimer)
  pressTimer = setTimeout(() => {
    selectMode.value = true
    suppressClickUntil = Date.now() + 800
    if (!selected.value.has(id)) toggleRow(id)
  }, 500)
}

function cancelPress() {
  clearTimeout(pressTimer)
}

function actionsFor(row: TData): ActionItem[] {
  return props.rowActions?.(row) ?? []
}

function rowNumberOnPage(index: number): number {
  return rowNumber(page.value, pageSize.value, index)
}

function activate(row: TData, id: string) {
  if (selectMode.value && isPhone.value) {
    toggleRow(id)
    return
  }
  if (props.clickable) emit('rowClick', row)
}

// Row activation skips events from interactive descendants: without the
// guard, Space or Enter on a row's checkbox or menu would both fire that
// control and open the row. (The select and actions cells also stop clicks.)
const interactiveSelector = 'button, a, input, select, textarea, [role="checkbox"], [role="menuitem"], [role="menuitemcheckbox"]'
function onRowClick(event: MouseEvent, row: TData, id: string) {
  if (Date.now() < suppressClickUntil) return
  const target = event.target as HTMLElement | null
  if (target?.closest(interactiveSelector)) return
  activate(row, id)
}
function onRowKeydown(event: KeyboardEvent, row: TData, id: string) {
  const target = event.target as HTMLElement | null
  if (target?.closest(interactiveSelector)) return
  activate(row, id)
}

// Resizable tables size from the TanStack column widths plus the fixed
// internal columns (select 40, row number 48, actions 48).
const tableWidth = computed(() => {
  if (!props.resizable) return undefined
  const fixed = (hasBulk.value ? 40 : 0) + (props.showRowNumbers ? 48 : 0) + (hasRowActions.value ? 48 : 0)
  return `${table.getTotalSize() + fixed}px`
})
const bodyColumnCount = computed(() => table.getAllColumns().length + (hasBulk.value ? 1 : 0) + (props.showRowNumbers ? 1 : 0) + (hasRowActions.value ? 1 : 0))
const activeFilterCount = computed(() => props.filters.reduce((count, filter) => count + filter.selected.length, 0) + (searchDraft.value.trim() !== '' ? 1 : 0))
</script>

<template>
  <div>
    <div v-if="showToolbar" class="mb-3 flex flex-wrap items-center gap-2">
      <template v-if="showSelectionBar">
        <p class="text-sm font-medium tabular-nums" aria-live="polite">{{ selectionLabel }}</p>
        <div class="hidden min-[640px]:contents">
          <ActionGroup :actions="bulkActions" density="row" aria-label="Selected rows" @press="emit('bulkAction', $event)" />
        </div>
        <ActionButton kind="quiet" density="row" label="Clear selection" icon="x" @press="clearSelection" />
      </template>
      <template v-else>
        <div v-if="showSearch" class="relative w-full min-w-40 flex-1 sm:max-w-[480px]">
          <Search class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" aria-hidden="true" />
          <Input v-model="searchDraft" type="text" :placeholder="searchPlaceholder" :aria-label="searchPlaceholder" class="pr-8! pl-9!" />
          <button
            v-if="searchDraft"
            type="button"
            aria-label="Clear search"
            class="text-muted-foreground hover:text-foreground absolute top-1/2 right-2 grid size-6 -translate-y-1/2 place-items-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring"
            @click="searchDraft = ''"
          >
            <X class="size-3.5" aria-hidden="true" />
          </button>
        </div>
        <div v-for="filter in filters" :key="filter.id" class="hidden min-[640px]:contents">
          <Popover>
            <PopoverTrigger as-child>
              <Button :variant="filter.selected.length > 0 ? 'secondary' : 'outline'" size="sm" class="max-w-64">
                <Plus v-if="filter.selected.length === 0" class="size-3.5" aria-hidden="true" />
                <span class="truncate">{{ filterSummary(filter) }}</span>
                <ChevronDown class="size-3.5 shrink-0 opacity-60" aria-hidden="true" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" class="w-56 p-1.5">
              <ul class="grid gap-0.5">
                <li v-for="option in filter.options" :key="option.value">
                  <label class="hover:bg-muted flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-2 text-sm">
                    <Checkbox :model-value="filter.selected.includes(option.value)" @update:model-value="toggleFilterValue(filter, option.value)" />
                    <span class="min-w-0 flex-1 truncate">{{ option.label }}</span>
                    <Check v-if="filter.selected.includes(option.value)" class="size-3.5 shrink-0" aria-hidden="true" />
                  </label>
                </li>
              </ul>
              <Button v-if="filter.selected.length > 0" variant="link" size="sm" class="mt-1 h-auto px-2 py-1" @click="setFilterSelected(filter.id, [])">
                Clear {{ filter.label.toLowerCase() }}
              </Button>
            </PopoverContent>
          </Popover>
        </div>
        <Button v-if="hasActiveFilters" variant="link" size="sm" class="hidden h-auto px-1 py-1 min-[640px]:inline-flex" @click="clearAllFilters">
          Clear filters
        </Button>
        <div class="ml-auto flex items-center gap-2">
          <ActionButton
            v-if="showFiltersButton"
            class="min-[640px]:hidden"
            kind="secondary"
            density="row"
            label="Filters"
            icon="list-filter"
            :counter="activeFilterCount > 0 ? activeFilterCount : null"
            @press="filtersOpen = true"
          />
          <div v-if="showColumnsButton" class="hidden min-[640px]:contents">
            <DropdownMenu>
              <DropdownMenuTrigger as-child>
                <Button variant="outline" size="sm">
                  Columns
                  <ChevronDown class="size-3.5 opacity-60" aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" class="w-52">
                <DropdownMenuCheckboxItem
                  v-for="column in toggleColumns"
                  :key="column.id"
                  :model-value="column.visible"
                  :disabled="!column.visible ? false : visibleDataCount <= 1"
                  :title="!column.visible || visibleDataCount > 1 ? undefined : 'At least one column stays visible.'"
                  @update:model-value="toggleColumn(column.id)"
                >
                  {{ column.label }}
                </DropdownMenuCheckboxItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <slot name="toolbar-actions" />
        </div>
      </template>
    </div>
    <div class="datatable-frame datatable-stacked overflow-auto rounded-lg border bg-card" :class="{ 'is-selecting': selectMode }">
      <Table :style="tableWidth ? { width: tableWidth } : undefined" :class="resizable ? 'datatable-grid' : undefined">
        <colgroup v-if="resizable">
          <col v-if="hasBulk" style="width: 40px" />
          <col v-if="showRowNumbers" style="width: 48px" />
          <col v-for="column in table.getAllLeafColumns()" :key="column.id" :style="{ width: `${column.getSize()}px` }" />
          <col v-if="hasRowActions" style="width: 48px" />
        </colgroup>
        <TableHeader>
          <TableRow v-for="headerGroup in table.getHeaderGroups()" :key="headerGroup.id" class="hover:bg-transparent">
            <TableHead v-if="hasBulk" class="datatable-select-cell w-10 px-0 text-center" aria-label="Select rows on this page">
              <Checkbox
                :model-value="allPageSelected ? true : somePageSelected ? 'indeterminate' : false"
                :aria-label="allPageSelected ? 'Clear the page selection' : 'Select every row on this page'"
                class="align-middle"
                @update:model-value="togglePage"
                @click.stop
              >
                <Minus v-if="!allPageSelected && somePageSelected" aria-hidden="true" />
                <Check v-else aria-hidden="true" />
              </Checkbox>
            </TableHead>
            <TableHead v-if="showRowNumbers" class="datatable-rownum-cell text-muted-foreground w-12 pr-4 text-right font-medium tabular-nums" aria-label="Row number">#</TableHead>
            <TableHead
              v-for="header in headerGroup.headers"
              :key="header.id"
              :colspan="header.colSpan"
              :class="['text-muted-foreground relative px-4 font-medium', numericColumn(header.column) ? 'text-right tabular-nums' : centerColumn(header.column) ? 'text-center' : undefined]"
            >
              <Button
                v-if="!header.isPlaceholder && header.column.getCanSort()"
                variant="ghost"
                size="sm"
                class="-ml-2"
                :aria-label="`Sort by ${stackLabel(header.column) || 'this column'}`"
                @click="header.column.getToggleSortingHandler()?.($event)"
              >
                <FlexRender :header="header" />
                <ArrowUp v-if="header.column.getIsSorted() === 'asc'" class="size-3.5" aria-hidden="true" />
                <ArrowDown v-else-if="header.column.getIsSorted() === 'desc'" class="size-3.5" aria-hidden="true" />
                <ArrowUpDown v-else class="size-3.5 opacity-50" aria-hidden="true" />
              </Button>
              <FlexRender v-else-if="!header.isPlaceholder" :header="header" />
              <span
                v-if="resizable && !header.isPlaceholder && header.column.getCanResize()"
                class="datatable-resizer"
                :class="{ 'is-resizing': header.column.getIsResizing() }"
                role="separator"
                aria-orientation="vertical"
                :aria-label="`Resize ${stackLabel(header.column) || 'this'} column`"
                title="Drag to resize, double-click to reset"
                @mousedown="header.getResizeHandler()($event)"
                @touchstart="header.getResizeHandler()($event)"
                @dblclick="header.column.resetSize()"
                @click.stop
              />
            </TableHead>
            <TableHead v-if="hasRowActions" class="datatable-actions-cell w-12 px-0" aria-label="Row actions" />
          </TableRow>
        </TableHeader>
        <TableBody>
          <template v-if="loading">
            <TableRow v-for="index in 5" :key="index" class="hover:bg-transparent">
              <TableCell v-if="hasBulk" class="datatable-select-cell px-0 text-center"><Skeleton class="mx-auto size-4" /></TableCell>
              <TableCell v-if="showRowNumbers" class="datatable-rownum-cell"><Skeleton class="ml-auto h-4 w-6" /></TableCell>
              <TableCell v-for="column in table.getAllColumns()" :key="column.id" class="pt-3 pr-4 pb-3 pl-4">
                <Skeleton class="h-5 w-full" />
              </TableCell>
              <TableCell v-if="hasRowActions" class="datatable-actions-cell px-0"><Skeleton class="mx-auto size-6" /></TableCell>
            </TableRow>
          </template>
          <TableEmpty v-else-if="error" :colspan="bodyColumnCount">
            <ErrorState title="These rows did not load" :message="error ?? 'Try again.'" @retry="emit('retry')" />
          </TableEmpty>
          <template v-else-if="pageRows.length > 0">
            <TableRow
              v-for="(row, index) in pageRows"
              :key="row.id"
              :tabindex="clickable && !(selectMode && isPhone) ? 0 : undefined"
              :data-state="isSelected(row.id) ? 'selected' : undefined"
              :class="[
                'hover:bg-[var(--app-surface-hover)] data-[state=selected]:bg-[var(--app-surface-selected)]',
                clickable && !(selectMode && isPhone) ? 'cursor-pointer focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none' : undefined,
              ]"
              @click="onRowClick($event, row.original as TData, row.id)"
              @keydown.enter="onRowKeydown($event, row.original as TData, row.id)"
              @keydown.space.prevent="onRowKeydown($event, row.original as TData, row.id)"
              @touchstart="onTouchStart(row.id)"
              @touchend="cancelPress"
              @touchmove="cancelPress"
              @contextmenu="isPhone && hasBulk ? $event.preventDefault() : undefined"
            >
              <TableCell v-if="hasBulk" data-label="" class="datatable-select-cell px-0 text-center" @click.stop>
                <Checkbox
                  :model-value="isSelected(row.id)"
                  :aria-label="`Select row ${rowNumberOnPage(index)}`"
                  class="align-middle"
                  @update:model-value="toggleRow(row.id)"
                  @click.stop
                />
              </TableCell>
              <TableCell v-if="showRowNumbers" data-label="" class="datatable-rownum-cell text-muted-foreground pr-4 text-right text-caption tabular-nums">
                <span class="datatable-rownum-full">{{ rowNumberOnPage(index) }}</span><span class="datatable-rownum-stacked" aria-hidden="true">#{{ rowNumberOnPage(index) }}</span>
              </TableCell>
              <TableCell
                v-for="cell in row.getAllCells()"
                :key="cell.id"
                :data-label="stackLabel(cell.column)"
                :class="['pt-3 pr-4 pb-3 pl-4', hiddenOnMobile(cell.column) ? 'datatable-hide-mobile' : undefined, numericColumn(cell.column) ? 'text-right tabular-nums' : centerColumn(cell.column) ? 'text-center' : undefined]"
              >
                <FlexRender :cell="cell" />
              </TableCell>
              <TableCell v-if="hasRowActions" data-label="" class="datatable-actions-cell px-0 text-center" @click.stop>
                <DropdownMenu v-if="actionsFor(row.original as TData).length > 0">
                  <DropdownMenuTrigger as-child>
                    <Button variant="ghost" size="icon" class="size-8" :aria-label="`Actions for row ${rowNumberOnPage(index)}`" title="Row actions">
                      <Ellipsis class="size-4" aria-hidden="true" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="min-w-44">
                    <template v-for="chunk in chunkByKind(actionsFor(row.original as TData))" :key="chunk.kind + chunk.actions[0]?.id">
                      <DropdownMenuSeparator v-if="chunk.dividerBefore" />
                      <DropdownMenuItem
                        v-for="action in chunk.actions"
                        :key="action.id"
                        :disabled="action.busy || !!action.disabledReason"
                        :title="action.disabledReason ?? undefined"
                        :class="chunk.kind === 'destructive' ? 'text-destructive focus:text-destructive' : undefined"
                        @select="emit('rowAction', action.id, row.original as TData)"
                      >
                        <component :is="action.icon ? icon(action.icon) : undefined" v-if="action.icon" class="size-4" aria-hidden="true" />
                        {{ action.label }}
                      </DropdownMenuItem>
                    </template>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          </template>
          <TableEmpty v-else-if="filtered || hasActiveFilters" :colspan="bodyColumnCount">
            <EmptyState :icon="emptyIcon" :art="filteredArt" title="No results for these filters" body="Try a different search, or clear the filters.">
              <template #action>
                <ActionButton kind="secondary" density="row" label="Clear filters" icon="x" @press="clearAllFilters" />
              </template>
            </EmptyState>
          </TableEmpty>
          <TableEmpty v-else :colspan="bodyColumnCount">
            <EmptyState :icon="emptyIcon" :art="emptyArt" :title="emptyTitle" :body="emptyBody">
              <template v-if="$slots['empty-action']" #action>
                <slot name="empty-action" />
              </template>
            </EmptyState>
          </TableEmpty>
        </TableBody>
      </Table>
    </div>
    <div class="mt-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
      <p class="text-muted-foreground text-sm tabular-nums" aria-live="polite">
        {{ selected.size > 0 ? selectionLabel : rangeLabel }}
      </p>
      <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
        <label class="text-muted-foreground flex items-center gap-2 text-sm">
          Rows per page
          <NativeSelect :model-value="String(pageSize)" size="sm" aria-label="Rows per page" @update:model-value="changePageSize(Number($event))">
            <NativeSelectOption v-for="option in PAGE_SIZE_OPTIONS" :key="option" :value="String(option)">{{ option }}</NativeSelectOption>
          </NativeSelect>
        </label>
        <p class="text-muted-foreground text-sm tabular-nums">{{ pageLabel }}</p>
        <div class="flex items-center gap-2">
          <ActionButton kind="secondary" density="row" label="Previous" icon="chevron-left" :disabled-reason="hasPrev ? null : 'You are on the first page.'" @press="prev" />
          <ActionButton kind="secondary" density="row" label="Next" icon="chevron-right" :disabled-reason="hasNext ? null : 'There are no more rows.'" @press="next" />
        </div>
      </div>
    </div>
    <BottomDrawer v-model:open="filtersOpen" title="Filters">
      <div v-if="sortableColumns.length > 0" class="grid gap-2">
        <p class="text-sm font-medium">Sort rows</p>
        <div class="flex items-center gap-2">
          <NativeSelect class="min-w-0 flex-1" :model-value="sortId" aria-label="Sort rows" @update:model-value="chooseSort(String($event))">
            <NativeSelectOption value="">No sorting</NativeSelectOption>
            <NativeSelectOption v-for="column in sortableColumns" :key="column.id" :value="column.id">
              {{ stackLabel(column) || column.id }}
            </NativeSelectOption>
          </NativeSelect>
          <ActionButton icon-only kind="secondary" :label="sortDesc ? 'Sort ascending' : 'Sort descending'" :icon="sortDesc ? 'arrow-down' : 'arrow-up'" :disabled-reason="!sortId ? 'Choose a column to sort.' : null" @press="flipDirection" />
        </div>
      </div>
      <div v-for="filter in filters" :key="filter.id" class="grid gap-2">
        <p class="text-sm font-medium">{{ filter.label }} <HelpHint v-if="filter.hint" :term="filter.hint" /></p>
        <ul class="grid gap-0.5">
          <li v-for="option in filter.options" :key="option.value">
            <label class="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-2.5 text-sm">
              <Checkbox :model-value="filter.selected.includes(option.value)" @update:model-value="toggleFilterValue(filter, option.value)" />
              <span class="min-w-0 flex-1">{{ option.label }}</span>
            </label>
          </li>
        </ul>
      </div>
      <div v-if="showColumnsButton" class="grid gap-2">
        <p class="text-sm font-medium">Columns</p>
        <ul class="grid gap-0.5">
          <li v-for="column in toggleColumns" :key="column.id">
            <label class="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-2.5 text-sm">
              <Checkbox
                :model-value="column.visible"
                :disabled="column.visible && visibleDataCount <= 1"
                :title="column.visible && visibleDataCount <= 1 ? 'At least one column stays visible.' : undefined"
                @update:model-value="toggleColumn(column.id)"
              />
              <span class="min-w-0 flex-1">{{ column.label }}</span>
            </label>
          </li>
        </ul>
      </div>
      <template #footer>
        <ActionButton v-if="hasActiveFilters" kind="quiet" label="Clear filters" icon="x" @press="clearAllFilters" />
        <ActionButton kind="primary" label="Done" icon="check" full-bleed @press="filtersOpen = false" />
      </template>
    </BottomDrawer>
    <div
      v-if="isPhone && showSelectionBar"
      class="bg-card fixed inset-x-3 z-30 flex items-center gap-2 rounded-[calc(var(--radius)*2)] border p-2 shadow-lg"
      style="bottom: calc(var(--space-16) + var(--space-3) + env(safe-area-inset-bottom, 0px))"
      role="toolbar"
      aria-label="Selected rows"
    >
      <p class="shrink-0 pl-1 text-sm font-medium tabular-nums">{{ selectionLabel }}</p>
      <div class="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
        <ActionButton
          v-for="action in bulkActions"
          :key="action.id"
          :kind="action.kind ?? 'secondary'"
          density="row"
          :label="action.label"
          :icon="action.icon"
          :busy="action.busy"
          :disabled-reason="action.disabledReason"
          @press="emit('bulkAction', action.id)"
        />
      </div>
      <ActionButton icon-only kind="quiet" density="row" label="Clear selection" icon="x" @press="clearSelection" />
    </div>
  </div>
</template>
