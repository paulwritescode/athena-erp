// The TanStack feature set behind DataTable, shared so column builders
// (Records, and later every table) type their definitions exactly as the
// grid reads them. Sorting orders client tables; sizing and resizing back
// the resizable grids, whose widths persist under their own keys.
import {
  columnResizingFeature,
  columnSizingFeature,
  createSortedRowModel,
  rowSortingFeature,
  sortFn_alphanumeric,
  tableFeatures,
  type ColumnDef,
  type RowData,
} from '@tanstack/vue-table'

export const dataTableFeatures = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: { alphanumeric: sortFn_alphanumeric },
  columnSizingFeature,
  columnResizingFeature,
})

export type DataTableColumn<T extends RowData> = ColumnDef<typeof dataTableFeatures, T, any>
