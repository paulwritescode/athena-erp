export type Page = 'overview' | 'record-types' | 'packs' | 'rules'
export type EnumChoice = { value: string; label: string }
// The optional attributes mirror RecordTypeField in proto/athena/v1/record_types.proto;
// the server enforces them and fieldRules.ts mirrors the checks. An absent
// attribute is not set.
export type Field = {
  name: string
  label: string
  type: string
  required: boolean
  target_record_type?: string
  unique?: boolean
  default?: string
  min?: string
  max?: string
  pattern?: string
  max_length?: number
  read_only?: boolean
  set_once?: boolean
  no_copy?: boolean
  allow_on_submit?: boolean
  index?: boolean
  values?: EnumChoice[]
  currency?: string
  uom?: string
  scale?: number
}
export type Kind = 'master' | 'document' | 'child' | 'setting'
// revision is 0 until the Record Type is first saved. class applies to documents
// and parent_record_type to child Record Types. submittable and amendable switch
// on the document lifecycle (draft, submitted, cancelled, amended).
export type RecordType = { name: string; label: string; description: string; kind: Kind; class?: string; parent_record_type?: string; submittable?: boolean; amendable?: boolean; fields: Field[]; revision: number }
