export type Page = 'overview' | 'record-types' | 'packs' | 'rules'
export type Field = { name: string; label: string; type: string; required: boolean; target_record_type?: string }
// revision is 0 until the Record Type is first saved.
export type RecordType = { name: string; label: string; description: string; fields: Field[]; revision: number }
