export type Page = 'overview' | 'record-types' | 'packs' | 'rules' | 'files' | 'access' | 'ledger' | 'reports' | 'jobs' | 'settings' | 'setup' | 'queue' | 'count' | 'gallery'
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
  // Expressions in the Athena expression language (ADR-0013). The server
  // type-checks them on save; formula is stored by the kernel on every write.
  formula?: string
  required_if?: string
  read_only_if?: string
  validate?: string
  depends_on?: string
  // Field permission level, 0 to 9 (ADR-0014). Users whose Grants reach a
  // lower level do not see the field.
  permission_level?: number
}
export type Kind = 'master' | 'document' | 'child' | 'setting'
// revision is 0 until the Record Type is first saved. class applies to documents
// and parent_record_type to child Record Types. submittable and amendable switch
// on the document lifecycle (draft, submitted, cancelled, amended). naming_series,
// gap_free and allow_manual_name set how a submittable document is named;
// label_field names the field that labels a Record in pickers.
export type RecordType = { name: string; label: string; description: string; kind: Kind; class?: string; parent_record_type?: string; submittable?: boolean; amendable?: boolean; naming_series?: string; gap_free?: boolean; allow_manual_name?: boolean; label_field?: string; fields: Field[]; revision: number; access?: RecordTypeAccess; record_count?: number; count_hidden?: boolean; updated_at?: string; updated_by?: string; pack_key?: string }

// What the signed-in user may do with a Record Type, as ListRecordTypes reports it.
export type RecordTypeAccess = { actions: string[]; permission_level: number }

// The signed-in user. systemPermissions holds build, access and audit.
export type SessionUser = { username: string; displayName: string; owner: boolean; systemPermissions: string[] }
