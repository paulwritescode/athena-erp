// Field editing shared by the Record Type editor table and the field
// settings sheet (F3): new-field tracking, label-to-key derivation, type
// changes, the totals composer over child numeric fields, and the F4
// condition-builder mapping for the sheet's Rules section.
import { reactive } from 'vue'
import { compileTree, type BuilderField, type ConditionTree, type TreeField } from '@/conditions'
import { snakeKey } from '@/components/record-types/recordTypeKeys'
import type { Field, RecordType } from '../types'

export const fieldTypes = ['Text', 'Long text', 'Email', 'Phone', 'Number', 'Currency', 'Money', 'Quantity', 'Date', 'Select', 'Enum', 'Link', 'Checkbox', 'Image']

// Fields added since the last save, and fields whose key the person typed by
// hand (keyed by object, so entries leave with their field).
const newFields = reactive(new Set<Field>())
const manuallyNamedFields = new WeakSet<Field>()

const reservedFieldKeys = ['id', 'revision', 'created_at', 'updated_at', 'created_by', 'modified_by', 'owner', 'deleted_at', 'def_version', 'request_id', 'parent_id', 'idx']

export function trackNewField(field: Field) {
  newFields.add(field)
}

export function forgetSavedFields(fields: Field[]) {
  for (const field of fields) newFields.delete(field)
}

export function isNewField(field: Field): boolean {
  return newFields.has(field)
}

export function fieldNameFromLabel(label: string, field: Field, siblings: Field[]): string {
  const taken = new Set(siblings.filter((item) => item !== field).map((item) => item.name.toLowerCase()))
  return snakeKey(label, 'field', taken, reservedFieldKeys)
}

export function updateFieldLabel(field: Field, label: string | number, siblings: Field[]) {
  field.label = String(label)
  if (newFields.has(field) && !manuallyNamedFields.has(field)) field.name = fieldNameFromLabel(field.label, field, siblings)
}

export function updateFieldName(field: Field, name: string | number) {
  field.name = String(name)
  manuallyNamedFields.add(field)
}

export function normalizeNewFieldName(field: Field, siblings: Field[]) {
  if (newFields.has(field)) field.name = fieldNameFromLabel(field.name, field, siblings)
}

// A new type drops every attribute but the carried ones, then takes its own
// defaults; Long text, Checkbox and Image cannot be unique or indexed.
export function changeType(field: Field) {
  const keep = new Set(['name', 'label', 'type', 'required', 'unique', 'index', 'read_only', 'set_once', 'no_copy'])
  for (const key of Object.keys(field)) if (!keep.has(key)) delete (field as Record<string, unknown>)[key]
  if (field.type === 'Money') { field.currency = 'USD'; field.scale = 2 }
  if (field.type === 'Quantity') field.uom = 'unit'
  if (field.type === 'Enum') field.values = [{ value: 'option_1', label: 'Option 1' }]
  if (field.type === 'Long text' || field.type === 'Checkbox' || field.type === 'Image') { delete field.unique; delete field.index }
}

export const totalFunctions = [
  { value: 'sum', label: 'Total' },
  { value: 'min', label: 'Smallest' },
  { value: 'max', label: 'Largest' },
  { value: 'count', label: 'Count' },
]

const decimalFieldTypes = new Set(['Number', 'Currency', 'Quantity'])

/** Field types a lines total can fill: decimals sum to decimals, money sums to money. */
export function isTotalTarget(type: string): boolean {
  return decimalFieldTypes.has(type) || type === 'Money'
}

/** Child Record Types belonging to the edited type. */
export function childTypesOf(recordTypes: RecordType[], currentName: string): RecordType[] {
  return recordTypes.filter((item) => item.kind === 'child' && item.parent_record_type === currentName)
}

/** The child fields a total into this field can read. */
export function totalSourceFields(fields: Field[], targetType: string): Field[] {
  return fields.filter((field) => targetType === 'Money' ? field.type === 'Money' : decimalFieldTypes.has(field.type))
}

/** sum(doc.lines.amount) from picks. */
export function composeTotal(fn: string, childKey: string, fieldKey: string): string {
  return `${fn}(doc.${childKey}.${fieldKey})`
}

/** Compile kind of a field's values; mirrors the server's exprKind and the Rules page mapping. */
export function builderKindFor(type: string): TreeField['kind'] {
  if (type === 'Number' || type === 'Currency' || type === 'Quantity') return 'decimal'
  if (type === 'Money') return 'money'
  if (type === 'Date') return 'date'
  if (type === 'Checkbox') return 'bool'
  return 'string'
}

/** Sibling fields as builder fields: doc-qualified names compile straight into field expressions. */
export function builderFieldsFor(fields: Field[]): BuilderField[] {
  return fields.map((f) => ({
    name: `doc.${f.name}`,
    label: f.label,
    type: f.type,
    kind: builderKindFor(f.type),
    currency: f.currency,
    scale: f.scale,
    choices: f.values?.map((c) => c.value),
  }))
}

/** Compiled expression for a builder tree, or '' when the tree is empty or unfinished. */
export function compileConditionTree(tree: ConditionTree | null, fields: BuilderField[]): string {
  if (!tree) return ''
  const vars: Record<string, TreeField> = {}
  for (const f of fields) vars[f.name] = { kind: f.kind, currency: f.currency, scale: f.scale }
  try {
    return compileTree(tree, vars)
  } catch {
    return ''
  }
}
