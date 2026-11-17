export type Page = 'overview' | 'records' | 'modules' | 'rules'
export type Field = { name: string; label: string; type: string; required: boolean; target_record?: string }
export type RecordDefinition = { name: string; label: string; description: string; fields: Field[] }
