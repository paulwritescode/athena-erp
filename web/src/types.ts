export type Page = 'overview' | 'forms' | 'modules' | 'rules'
export type Field = { name: string; label: string; type: string; required: boolean }
export type FormDefinition = { name: string; label: string; description: string; module: string; fields: Field[] }
