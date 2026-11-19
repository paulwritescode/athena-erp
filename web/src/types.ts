export type Page = 'overview' | 'records' | 'modules' | 'rules'
export type Field = { name: string; label: string; type: string; required: boolean; target_record?: string }
// revision is 0 until the Record is first saved.
export type RecordDefinition = { name: string; label: string; description: string; fields: Field[]; revision: number }
