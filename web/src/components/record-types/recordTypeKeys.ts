// Naming rules for new Record Types, shared by the create dialog and the
// editor. Keys derive from the label the way the server expects them.
import { gloss, type GlossaryKey } from '@/copy/glossary'
import type { Kind } from '@/types'

const KIND_GLOSSARY: Array<{ value: Kind; term: GlossaryKey }> = [
  { value: 'master', term: 'kindMaster' },
  { value: 'document', term: 'kindDocument' },
  { value: 'setting', term: 'kindSetting' },
  { value: 'child', term: 'kindChild' },
]

export const KIND_OPTIONS: Array<{ value: Kind; label: string; hint: string }> = KIND_GLOSSARY.map(({ value, term }) => ({ value, label: gloss(term).label, hint: gloss(term).explanation }))

export function snakeKey(text: string, fallback: string, taken: Set<string>, reserved: string[]): string {
  let base = text.normalize('NFKD').replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '')
  if (!base) base = fallback
  if (reserved.includes(base)) base = `${fallback}_${base}`
  if (/^[0-9]/.test(base)) base = `${fallback}_${base}`
  let candidate = base
  let suffix = 2
  while (taken.has(candidate)) candidate = `${base}_${suffix++}`
  return candidate
}

// deriveRecordTypeKey turns a label into the key a new Record Type saves
// under, unique among taken (lowercase) names.
export function deriveRecordTypeKey(label: string, taken: readonly string[]): string {
  return snakeKey(label, 'record_type', new Set(taken), [])
}

export type NewTypeErrors = { name?: string; key?: string }

// validateNewRecordType checks a dialog entry: the name is required and
// unique, the key fits the server rule and is unused.
export function validateNewRecordType(name: string, key: string, takenNames: readonly string[], takenLabels: readonly string[]): NewTypeErrors {
  const errors: NewTypeErrors = {}
  const trimmed = name.trim()
  if (!trimmed) errors.name = 'Enter a name.'
  else if (trimmed.length > 255) errors.name = 'Keep the name under 255 characters.'
  else if (takenLabels.includes(trimmed.toLowerCase())) errors.name = 'That name is taken. Try another.'
  if (!/^[a-z][a-z0-9_]{0,49}$/.test(key)) errors.key = 'Use lowercase letters, numbers and underscores, starting with a letter.'
  else if (takenNames.includes(key)) errors.key = 'That key is taken. Try another.'
  return errors
}
