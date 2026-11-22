// Pure list model for the Record Types list: filtering, sort values and
// labels. The DataTable owns sort state; its columns read sort values from
// here so the suite pins the ordering.
import type { Kind, RecordType } from '@/types'

export const kindOptions: Array<{ value: Kind | 'all'; label: string }> = [
  { value: 'all', label: 'All kinds' },
  { value: 'master', label: 'Master' },
  { value: 'document', label: 'Document' },
  { value: 'child', label: 'Child' },
  { value: 'setting', label: 'Setting' },
]

export function kindLabel(kind: string): string {
  return kindOptions.find((option) => option.value === kind)?.label ?? kind
}

export function packLabel(packKey: string | undefined): string {
  return packKey || 'None'
}

export function filterRecordTypes(types: RecordType[], search: string, kinds: string[], packs: string[]): RecordType[] {
  const needle = search.trim().toLowerCase()
  return types.filter((type) => {
    if (kinds.length > 0 && !kinds.includes(type.kind || 'master')) return false
    if (packs.length > 0 && !packs.includes(type.pack_key || 'none')) return false
    if (!needle) return true
    return `${type.label} ${type.name} ${type.description}`.toLowerCase().includes(needle)
  })
}

// Sort values per list column. Hidden counts sort below zero so unknown
// sizes group at one end; ISO timestamps sort chronologically as text.
export const listSortValues: Record<string, (type: RecordType) => string | number> = {
  name: (type) => `${type.label || type.name}`.toLowerCase(),
  description: (type) => type.description.toLowerCase(),
  kind: (type) => kindLabel(type.kind || 'master'),
  fields: (type) => type.fields.length,
  records: (type) => (type.count_hidden ? -1 : (type.record_count ?? 0)),
  pack: (type) => type.pack_key || '',
  updated: (type) => type.updated_at || '',
  changedBy: (type) => (type.updated_by || '').toLowerCase(),
}
