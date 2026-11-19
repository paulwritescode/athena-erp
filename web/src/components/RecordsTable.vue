<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { columnResizingFeature, columnSizingFeature, tableFeatures, useTable, type ColumnDef, type ColumnSizingState, type Updater } from '@tanstack/vue-table'
import type { Record as RecordMessage } from '../gen/athena/v1/records_pb'
import type { Field } from '../types'
import { labelWithState, resolveRecords } from '../lookup'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const props = defineProps<{ recordTypeKey: string; fields: Field[]; records: RecordMessage[]; submittable?: boolean }>()
const emit = defineEmits<{ open: [record: RecordMessage] }>()

const features = tableFeatures({ columnSizingFeature, columnResizingFeature })

// Starting widths in pixels by field type; people can drag any column wider.
const defaultWidths: Record<string, number> = { 'Long text': 320, Email: 240, Text: 200, Select: 160, Link: 200, Phone: 160, Date: 140, Number: 130, Currency: 130, Money: 150, Quantity: 150, Enum: 160, Checkbox: 110 }
const alignRight = new Set(['Number', 'Currency', 'Money', 'Quantity'])
const alignCenter = new Set(['Checkbox'])

// Link cells show the linked Record's label, never its ID. Labels load once
// per page of Records and fill in as they arrive.
const linkLabels = ref<Record<string, string>>({})
watch(() => props.records, async (records) => {
  for (const field of props.fields) {
    if (field.type !== 'Link' || !field.target_record_type) continue
    const ids = records.flatMap((record) => { const kind = record.values[field.name]?.kind; return kind?.case === 'stringValue' ? [kind.value] : [] })
    if (ids.length === 0) continue
    try {
      const found = await resolveRecords(field.target_record_type, ids)
      linkLabels.value = { ...linkLabels.value, ...Object.fromEntries([...found].map(([id, item]) => [id, labelWithState(item)])) }
    } catch {
      // The cell stays empty; opening the Record shows the error toast if the lookup keeps failing.
    }
  }
}, { immediate: true })
function display(record: RecordMessage, field: Field): string {
  const kind = record.values[field.name]?.kind
  if (kind?.case === 'boolValue') return kind.value ? 'Yes' : 'No'
  if (kind?.case === 'stringValue') return field.type === 'Enum' ? field.values?.find((choice) => choice.value === kind.value)?.label ?? kind.value : kind.value
  return ''
}

// Fields only change when the Record Type is saved; the parent re-creates this
// component per Record Type, so the column list is stable for a table's lifetime.
const stateLabels = ['Draft', 'Submitted', 'Cancelled']
// A Link cell holds the linked Record's ID in the table data; the text shown
// (and the tooltip) is its label once that has loaded, so an ID is never shown.
const linkColumns = computed(() => new Set(props.fields.filter((field) => field.type === 'Link').map((field) => field.name)))
function cellText(columnId: string, value: unknown): string {
  const text = String(value ?? '')
  return linkColumns.value.has(columnId) ? linkLabels.value[text] ?? '' : text
}
const columns = computed<ColumnDef<typeof features, RecordMessage>[]>(() => [
  ...(props.submittable ? [{ id: 'docstate', header: 'Status', accessorFn: (record: RecordMessage) => stateLabels[record.docstate] ?? '', size: 120, minSize: 80, maxSize: 300 } satisfies ColumnDef<typeof features, RecordMessage>] : []),
  ...props.fields.map((field): ColumnDef<typeof features, RecordMessage> => ({
    id: field.name,
    header: field.type === 'Money' && field.currency ? `${field.label} (${field.currency})` : field.type === 'Quantity' && field.uom ? `${field.label} (${field.uom})` : field.label,
    accessorFn: (record: RecordMessage) => display(record, field),
    size: defaultWidths[field.type] ?? 180,
    minSize: 80,
    maxSize: 900,
  })),
])

const storageKey = computed(() => `athena-entry-columns:${props.recordTypeKey}`)

function loadSizing(): ColumnSizingState {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(storageKey.value) ?? '{}')
    if (!parsed || typeof parsed !== 'object') return {}
    return Object.fromEntries(Object.entries(parsed).filter((pair): pair is [string, number] => typeof pair[1] === 'number' && Number.isFinite(pair[1])))
  } catch {
    return {}
  }
}

const columnSizing = ref<ColumnSizingState>(loadSizing())
watch(columnSizing, (value) => { localStorage.setItem(storageKey.value, JSON.stringify(value)) }, { deep: true })

const table = useTable({
  features,
  columns,
  data: computed(() => props.records),
  getRowId: (record: RecordMessage) => record.id,
  columnResizeMode: 'onChange',
  state: computed(() => ({ columnSizing: columnSizing.value })),
  onColumnSizingChange: (next: Updater<ColumnSizingState>) => {
    columnSizing.value = typeof next === 'function' ? next(columnSizing.value) : next
  },
})

const fieldTypes = computed(() => new Map(props.fields.map((field) => [field.name, field.type])))

function align(columnId: string) {
  const type = fieldTypes.value.get(columnId) ?? ''
  return alignRight.has(type) ? 'is-right' : alignCenter.has(type) ? 'is-center' : ''
}
</script>

<template>
  <div class="entries-scroll">
    <Table class="entries-grid" :style="{ width: `${table.getTotalSize()}px` }">
      <colgroup>
        <col v-for="column in table.getAllLeafColumns()" :key="column.id" :style="{ width: `${column.getSize()}px` }" />
      </colgroup>
      <TableHeader>
        <TableRow v-for="group in table.getHeaderGroups()" :key="group.id">
          <TableHead v-for="header in group.headers" :key="header.id" :class="align(header.column.id)" :title="String(header.column.columnDef.header ?? '')">
            <span class="entries-head-label">{{ header.column.columnDef.header }}</span>
            <span
              class="entries-resizer"
              :class="{ 'is-resizing': header.column.getIsResizing() }"
              role="separator"
              aria-orientation="vertical"
              :aria-label="`Resize ${String(header.column.columnDef.header ?? '')} column`"
              title="Drag to resize, double-click to reset"
              @mousedown="header.getResizeHandler()($event)"
              @touchstart="header.getResizeHandler()($event)"
              @dblclick="header.column.resetSize()"
              @click.stop
            />
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="row in table.getRowModel().rows" :key="row.id" tabindex="0" class="entries-row" @click="emit('open', row.original)" @keydown.enter="emit('open', row.original)">
          <TableCell v-for="cell in row.getAllCells()" :key="cell.id" :class="align(cell.column.id)" :title="cellText(cell.column.id, cell.getValue())">{{ cellText(cell.column.id, cell.getValue()) }}</TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>
