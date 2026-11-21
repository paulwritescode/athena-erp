// Record columns for the Records table: one TanStack definition per field,
// a Status badge for submittable Record Types, and the primary column (the
// label field, else the first field) as a PrimaryCell with the update time
// below. Link cells show the linked Record's label, never its ID. Pure
// except for the cell renderers, so the suite pins the text and shape.
import { h } from 'vue'
import PrimaryCell from '@/components/PrimaryCell.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { DataTableColumn } from '@/components/dataTableFeatures'
import type { DataTableColumnMeta } from '@/components/DataTable.vue'
import { gloss } from '@/copy/glossary'
import type { Record as RecordMessage } from '@/gen/athena/v1/records_pb'
import type { Field } from '@/types'

const stateKeys = ['draft', 'submitted', 'cancelled'] as const
export const stateLabels = stateKeys.map((key) => gloss(key).label)

// Starting widths in pixels by field type; people can drag any column wider.
const defaultWidths: Record<string, number> = {
  'Long text': 320, Email: 240, Text: 200, Select: 160, Link: 200, Phone: 160,
  Date: 140, Number: 130, Currency: 130, Money: 150, Quantity: 150, Enum: 160, Checkbox: 110,
}
const alignRight = new Set(['Number', 'Currency', 'Money', 'Quantity'])

export function recordFieldText(field: Field, record: RecordMessage, linkLabels: Record<string, string>): string {
  const kind = record.values[field.name]?.kind
  if (kind?.case === 'boolValue') return kind.value ? 'Yes' : 'No'
  if (kind?.case !== 'stringValue') return ''
  if (field.type === 'Enum') return field.values?.find((choice) => choice.value === kind.value)?.label ?? kind.value
  if (field.type === 'Link') return linkLabels[kind.value] ?? ''
  return kind.value
}

export function recordFieldHeader(field: Field): string {
  if (field.type === 'Money' && field.currency) return `${field.label} (${field.currency})`
  if (field.type === 'Quantity' && field.uom) return `${field.label} (${field.uom})`
  return field.label
}

// The Status filter and the search box combine into one server filter: the
// user's expression and the chosen states, each parenthesized. Empty in,
// empty out.
export function combineRecordFilter(userFilter: string, docstates: number[]): string {
  const parts: string[] = []
  const expression = userFilter.trim()
  if (expression) parts.push(`(${expression})`)
  const states = [...new Set(docstates)].sort()
  if (states.length === 1) parts.push(`(doc.docstate == ${states[0]})`)
  else if (states.length > 1) parts.push(`(${states.map((state) => `doc.docstate == ${state}`).join(' || ')})`)
  return parts.join(' && ')
}

export type RecordColumnsInput = {
  fields: Field[]
  submittable: boolean
  linkLabels: Record<string, string>
  primaryField?: string
  updatedLine: (record: RecordMessage) => string
}

export function buildRecordColumns(input: RecordColumnsInput): DataTableColumn<RecordMessage>[] {
  const primary = input.fields.some((field) => field.name === input.primaryField)
    ? (input.primaryField as string)
    : (input.fields[0]?.name ?? '')
  const columns: DataTableColumn<RecordMessage>[] = []
  if (input.submittable) {
    columns.push({
      id: 'docstate',
      header: 'Status',
      accessorFn: (record) => stateLabels[record.docstate] ?? '',
      cell: ({ row }) => h(StatusBadge, { status: stateKeys[row.original.docstate] ?? '', label: stateLabels[row.original.docstate] ?? '' }),
      meta: { label: 'Status' },
      size: 130,
      minSize: 80,
      maxSize: 300,
      enableSorting: false,
    })
  }
  for (const [index, field] of input.fields.entries()) {
    const header = recordFieldHeader(field)
    const meta: DataTableColumnMeta = {
      label: field.label,
      hideOnMobile: index > 1,
      numeric: alignRight.has(field.type),
      center: field.type === 'Checkbox',
    }
    if (field.name === primary) {
      columns.push({
        id: field.name,
        header,
        accessorFn: (record) => recordFieldText(field, record, input.linkLabels),
        cell: ({ row }) => h(PrimaryCell, {
          title: recordFieldText(field, row.original, input.linkLabels) || 'Untitled',
          subtitle: input.updatedLine(row.original),
        }),
        meta,
        size: defaultWidths[field.type] ?? 180,
        minSize: 80,
        maxSize: 900,
        enableSorting: false,
      })
    } else {
      columns.push({
        id: field.name,
        header,
        accessorFn: (record) => recordFieldText(field, record, input.linkLabels),
        cell: ({ row }) => {
          const text = recordFieldText(field, row.original, input.linkLabels)
          return h('span', { title: text || undefined }, text)
        },
        meta,
        size: defaultWidths[field.type] ?? 180,
        minSize: 80,
        maxSize: 900,
        enableSorting: false,
      })
    }
  }
  return columns
}
