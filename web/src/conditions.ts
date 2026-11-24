// Condition trees for the F4 no-code builder: the JSON model the Desk edits
// plus a compile mirror of Go's internal/expr CompileTree for live
// validation ("Show as expression" while typing). The server recompiles
// authoritatively on save; this file must produce byte-identical output
// for valid trees (conditions.test.ts pins representative cases against
// the Go suite's expectations). No parse-back here: the server owns loads.
export type ConditionOp =
  | 'is' | 'is_not'
  | 'contains' | 'not_contains' | 'starts_with'
  | 'is_empty' | 'is_not_empty'
  | 'more_than' | 'at_least' | 'less_than' | 'at_most'
  | 'is_any_of' | 'is_none_of'
  | 'before' | 'after' | 'on_or_before' | 'on_or_after'
  | 'in_the_last_n_days' | 'in_the_next_n_days'
  | 'is_checked' | 'is_not_checked'

export interface TreeCondition {
  field: string
  op: ConditionOp
  values?: string[]
}

export interface TreeGroup {
  all: boolean
  conditions?: TreeCondition[]
  groups?: TreeGroup[]
}

export interface ConditionTree {
  root: TreeGroup
}

export type TreeFieldKind = 'string' | 'decimal' | 'money' | 'date' | 'bool' | 'int'

export interface TreeField {
  kind: TreeFieldKind
  currency?: string
  scale?: number
}

// BuilderField is one field the builder offers: the variable name the model
// stores, the label it shows, the Athena field type for operator narrowing,
// the compile kind, and optional value choices for autocomplete.
export interface BuilderField {
  name: string
  label: string
  type: string
  kind: TreeFieldKind
  currency?: string
  scale?: number
  choices?: string[]
}

// Plain-words labels for the operator select. Kept beside the compiler so
// labels and behavior change together.
export const OPERATOR_LABELS: Record<ConditionOp, string> = {
  is: 'is',
  is_not: 'is not',
  contains: 'contains',
  not_contains: 'does not contain',
  starts_with: 'starts with',
  is_empty: 'is empty',
  is_not_empty: 'is not empty',
  more_than: 'more than',
  at_least: 'at least',
  less_than: 'less than',
  at_most: 'at most',
  is_any_of: 'is any of',
  is_none_of: 'is none of',
  before: 'before',
  after: 'after',
  on_or_before: 'on or before',
  on_or_after: 'on or after',
  in_the_last_n_days: 'in the last',
  in_the_next_n_days: 'in the next',
  is_checked: 'is checked',
  is_not_checked: 'is not checked',
}

// Operator lists per Athena field type for the builder UI. The compiler
// below is kind-level (like Go); the UI narrows by field type (Enum gets
// no contains family, Link gets is/empty only, documents add Status).
export const OPERATORS_FOR_TYPE: Record<string, ConditionOp[]> = {
  Text: ['is', 'is_not', 'contains', 'not_contains', 'starts_with', 'is_empty', 'is_not_empty'],
  Email: ['is', 'is_not', 'contains', 'not_contains', 'starts_with', 'is_empty', 'is_not_empty'],
  Phone: ['is', 'is_not', 'contains', 'not_contains', 'starts_with', 'is_empty', 'is_not_empty'],
  'Long text': ['is', 'is_not', 'contains', 'not_contains', 'starts_with', 'is_empty', 'is_not_empty'],
  Number: ['is', 'is_not', 'more_than', 'at_least', 'less_than', 'at_most', 'is_empty', 'is_not_empty'],
  Currency: ['is', 'is_not', 'more_than', 'at_least', 'less_than', 'at_most', 'is_empty', 'is_not_empty'],
  Quantity: ['is', 'is_not', 'more_than', 'at_least', 'less_than', 'at_most', 'is_empty', 'is_not_empty'],
  Money: ['is', 'is_not', 'more_than', 'at_least', 'less_than', 'at_most', 'is_empty', 'is_not_empty'],
  Date: ['is', 'is_not', 'before', 'after', 'on_or_before', 'on_or_after', 'in_the_last_n_days', 'in_the_next_n_days', 'is_empty', 'is_not_empty'],
  Checkbox: ['is_checked', 'is_not_checked'],
  Enum: ['is', 'is_not', 'is_any_of', 'is_none_of', 'is_empty', 'is_not_empty'],
  Select: ['is', 'is_not', 'is_any_of', 'is_none_of', 'is_empty', 'is_not_empty'],
  Link: ['is', 'is_not', 'is_empty', 'is_not_empty'],
  Status: ['is', 'is_not', 'is_empty', 'is_not_empty'],
}

const MAX_DIGITS = 65

function quoteCEL(s: string): string {
  return `'${s.replace(/\\/g, '\\\\').replace(/'/g, `\\'`)}'`
}

// quoteMeta mirrors Go's regexp.QuoteMeta byte for byte: ASCII bytes
// outside [A-Za-z0-9_] take a backslash, everything else passes through.
function quoteMeta(s: string): string {
  let out = ''
  for (const ch of s) {
    const code = ch.codePointAt(0) ?? 0
    if (code < 128 && !/[A-Za-z0-9_]/.test(ch)) out += `\\${ch}`
    else out += ch
  }
  return out
}

// canonDecimal mirrors types.ParseDecimal + Decimal.String: strict
// optional sign + digits[.digits], no exponents or spaces, scale kept.
function canonDecimal(text: string): string | null {
  let body = text
  let neg = false
  if (body.startsWith('-')) {
    neg = true
    body = body.slice(1)
  } else if (body.startsWith('+')) {
    body = body.slice(1)
  }
  const dot = body.indexOf('.')
  if (dot !== body.lastIndexOf('.')) return null
  const whole = dot < 0 ? body : body.slice(0, dot)
  const frac = dot < 0 ? '' : body.slice(dot + 1)
  if (whole === '' && frac === '') return null
  if (dot >= 0 && frac === '') return null
  if (!/^\d*$/.test(whole) || !/^\d*$/.test(frac)) return null
  let digits = (whole + frac).replace(/^0+/, '')
  if (digits === '') digits = '0'
  if (digits.length > MAX_DIGITS || frac.length > MAX_DIGITS) return null
  let out = digits
  if (frac.length > 0) {
    while (out.length <= frac.length) out = `0${out}`
    out = `${out.slice(0, out.length - frac.length)}.${out.slice(out.length - frac.length)}`
  }
  if (neg && !/^0(\.0*)?$/.test(out)) out = `-${out}`
  return out
}

function canonInt(text: string): string | null {
  if (!/^-?\d+$/.test(text)) return null
  try {
    const n = BigInt(text)
    if (n < -(1n << 63n) || n > ((1n << 63n) - 1n)) return null
    return n.toString(10)
  } catch {
    return null
  }
}

function canonDay(text: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(text)) return null
  const [y, m, d] = text.split('-').map(Number)
  const day = new Date(Date.UTC(y, m - 1, d))
  if (day.getUTCFullYear() !== y || day.getUTCMonth() !== m - 1 || day.getUTCDate() !== d) return null
  return text
}

function one(c: TreeCondition): string {
  if ((c.values ?? []).length !== 1) throw new Error(`${c.op} on ${c.field} needs exactly one value`)
  return (c.values ?? [])[0] as string
}

const COMPARE: Partial<Record<ConditionOp, string>> = {
  is: '==', is_not: '!=', more_than: '>', at_least: '>=', less_than: '<', at_most: '<=',
}

const DATE_COMPARE: Partial<Record<ConditionOp, string>> = {
  is: '==', is_not: '!=', before: '<', after: '>', on_or_before: '<=', on_or_after: '>=',
}

function compileCondition(c: TreeCondition, fields: Record<string, TreeField>): string {
  const v = fields[c.field]
  if (!v) throw new Error(`unknown field "${c.field}"`)
  const f = c.field
  const emptyOp = (op: ConditionOp): string | null => {
    if (c.op !== op) return null
    if ((c.values ?? []).length !== 0) throw new Error(`${c.op} on ${c.field} takes no value`)
    return op === 'is_empty' ? `${f} == null` : `${f} != null`
  }
  if (c.op === 'is_empty' || c.op === 'is_not_empty') {
    if (v.kind === 'bool') throw new Error(`${c.op} cannot be used on "${c.field}"`)
    return emptyOp(c.op) as string
  }
  switch (v.kind) {
    case 'string': {
      switch (c.op) {
        case 'is':
        case 'is_not': {
          const op = c.op === 'is' ? '==' : '!='
          return `${f} ${op} ${quoteCEL(one(c))}`
        }
        case 'contains':
        case 'not_contains': {
          const call = `${f}.matches(${quoteCEL(`(?s).*${quoteMeta(one(c))}.*`)})`
          return c.op === 'contains' ? call : `!(${call})`
        }
        case 'starts_with':
          return `${f}.startsWith(${quoteCEL(one(c))})`
        case 'is_any_of':
        case 'is_none_of': {
          const values = c.values ?? []
          if (values.length < 1) throw new Error(`${c.op} on ${c.field} needs at least one value`)
          if (c.op === 'is_any_of') return `(${values.map((x) => `${f} == ${quoteCEL(x)}`).join(' || ')})`
          return `(${values.map((x) => `${f} != ${quoteCEL(x)}`).join(' && ')})`
        }
      }
      break
    }
    case 'decimal': {
      const op = COMPARE[c.op]
      if (!op) break
      const dec = canonDecimal(one(c))
      if (dec === null) throw new Error(`${c.field}: "${one(c)}" is not a number`)
      return `${f} ${op} dec('${dec}')`
    }
    case 'money': {
      if (!v.currency) throw new Error(`${c.field} has no currency`)
      const scale = v.scale ?? 0
      const op = COMPARE[c.op]
      if (!op) break
      const raw = one(c)
      const dec = canonDecimal(raw)
      if (dec === null) throw new Error(`${c.field}: "${raw}" is not a number`)
      const fracLen = dec.includes('.') ? (dec.split('.')[1] as string).length : 0
      if (fracLen > scale) throw new Error(`${c.field}: "${raw}" does not fit ${v.currency}`)
      return `${f} ${op} money(dec('${dec}'), '${v.currency}', ${scale})`
    }
    case 'date': {
      const op = DATE_COMPARE[c.op]
      if (op) {
        const day = canonDay(one(c))
        if (day === null) throw new Error(`${c.field}: "${one(c)}" is not a YYYY-MM-DD date`)
        return `${f} ${op} date('${day}')`
      }
      if (c.op === 'in_the_last_n_days' || c.op === 'in_the_next_n_days') {
        const raw = one(c)
        if (!/^\d+$/.test(raw)) throw new Error(`${c.field}: "${raw}" is not a day count of 0 or more`)
        const n = Number(raw)
        if (c.op === 'in_the_last_n_days') return `(${f} >= add_days(today(), ${-n}) && ${f} <= today())`
        return `(${f} >= today() && ${f} <= add_days(today(), ${n}))`
      }
      break
    }
    case 'bool': {
      if (c.op === 'is_checked' || c.op === 'is_not_checked') {
        if ((c.values ?? []).length !== 0) throw new Error(`${c.op} on ${c.field} takes no value`)
        return c.op === 'is_checked' ? `${f} == true` : `${f} == false`
      }
      break
    }
    case 'int': {
      const op = COMPARE[c.op]
      if (!op) break
      const raw = one(c)
      const n = canonInt(raw)
      if (n === null) throw new Error(`${c.field}: "${raw}" is not a whole number`)
      return `${f} ${op} ${n}`
    }
  }
  throw new Error(`${c.op} cannot be used on "${c.field}"`)
}

function compileGroup(g: TreeGroup, fields: Record<string, TreeField>, root: boolean): string {
  const conditions = g.conditions ?? []
  const groups = g.groups ?? []
  const n = conditions.length + groups.length
  if (root && n < 1) throw new Error('the builder needs at least one condition')
  if (!root && n < 2) throw new Error('a nested group needs at least two conditions')
  const parts = [
    ...conditions.map((c) => compileCondition(c, fields)),
    ...groups.map((child) => compileGroup(child, fields, false)),
  ]
  const out = parts.join(g.all ? ' && ' : ' || ')
  return root && parts.length === 1 ? out : `(${out})`
}

// compileTree renders the canonical expression. Throws Error naming the
// field on unknown fields, wrong operators and malformed values.
export function compileTree(tree: ConditionTree, fields: Record<string, TreeField>): string {
  return compileGroup(tree.root, fields, true)
}
