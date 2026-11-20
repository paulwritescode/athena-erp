// Pure helpers for the Users & Roles desk: Scope parsing, assignment diffs
// and access previews. The server re-checks everything; these only shape
// what the administrator sees before saving.

export type ScopeMap = Record<string, string[]>

// Bounds mirror internal/auth assignment.go.
export const maxScopeDimensions = 32
export const maxScopeValues = 256
export const maxScopeValueBytes = 256
const dimensionPattern = /^[a-z][a-z0-9_]*$/

export type ParsedScope = { scope: ScopeMap; problems: string[] }

// parseScope reads an Assignment Scope in dimension=value,... form, with
// dimensions separated by semicolons: company=acme,globex;branch=north. An
// empty text is the empty Scope. Problems name every violation; the scope
// holds what parsed.
export function parseScope(text: string): ParsedScope {
  const scope: ScopeMap = {}
  const problems: string[] = []
  if (!text.trim()) return { scope, problems }
  const encoder = new TextEncoder()
  const parts = text.split(';')
  if (parts.length > maxScopeDimensions) problems.push(`A Scope holds at most ${maxScopeDimensions} dimensions.`)
  for (const part of parts) {
    const at = part.indexOf('=')
    const dimension = (at < 0 ? part : part.slice(0, at)).trim()
    if (at < 0 || !dimensionPattern.test(dimension)) {
      problems.push(`Scope dimension ${JSON.stringify(dimension || part)} is not a field key.`)
      continue
    }
    const values = part
      .slice(at + 1)
      .split(',')
      .map((value) => value.trim())
    if (values.length > maxScopeValues) problems.push(`Scope dimension ${dimension} holds at most ${maxScopeValues} values.`)
    for (const value of values) {
      if (value.length === 0 || encoder.encode(value).length > maxScopeValueBytes) {
        problems.push(`A Scope value needs 1 to ${maxScopeValueBytes} bytes.`)
        break
      }
    }
    scope[dimension] = values
  }
  return { scope, problems }
}

// formatScope renders a Scope in the parseScope form, deterministically.
export function formatScope(scope: ScopeMap): string {
  return Object.keys(scope)
    .sort()
    .map((dimension) => `${dimension}=${[...scope[dimension]].sort().join(',')}`)
    .join(';')
}

// describeScope reads a Scope aloud for previews and chips.
export function describeScope(scope: ScopeMap): string {
  const dimensions = Object.keys(scope).sort()
  if (dimensions.length === 0) return 'Everywhere'
  const title = (text: string) => text.slice(0, 1).toUpperCase() + text.slice(1)
  return dimensions.map((dimension) => `${title(dimension)} ${[...scope[dimension]].sort().join(', ')}`).join(' · ')
}

export type AssignmentLike = { role: string; scope: ScopeMap; activeFrom: string; expiresAt: string }

// diffAssignments lists how an Assignment changes, one human line per
// difference. An empty list means no changes.
export function diffAssignments(oldA: AssignmentLike, next: AssignmentLike): string[] {
  const lines: string[] = []
  if (oldA.role !== next.role) lines.push(`Role: ${oldA.role || '(none)'} → ${next.role || '(none)'}.`)
  const bound = (text: string) => (text ? text : 'unbounded')
  if (oldA.activeFrom !== next.activeFrom) lines.push(`Active from: ${bound(oldA.activeFrom)} → ${bound(next.activeFrom)}.`)
  if (oldA.expiresAt !== next.expiresAt) lines.push(`Expires: ${bound(oldA.expiresAt)} → ${bound(next.expiresAt)}.`)
  const dims = new Set([...Object.keys(oldA.scope), ...Object.keys(next.scope)])
  for (const dimension of [...dims].sort()) {
    const was = oldA.scope[dimension]
    const is = next.scope[dimension]
    if (was === undefined && is !== undefined) {
      lines.push(`Scope gains ${dimension}=${is.join(',')}.`)
    } else if (was !== undefined && is === undefined) {
      lines.push(`Scope loses ${dimension}.`)
    } else if (was !== undefined && is !== undefined) {
      for (const value of is.filter((item) => !was.includes(item)).sort()) lines.push(`Scope ${dimension} gains ${value}.`)
      for (const value of was.filter((item) => !is.includes(item)).sort()) lines.push(`Scope ${dimension} loses ${value}.`)
    }
  }
  return lines
}

// actionLetters compresses Grant actions into the permissions matrix cells:
// Read, Create, Write, Delete, Submit, cancel (X), Amend, audit Versions.
const actionLetters: Record<string, string> = { read: 'R', create: 'C', write: 'W', delete: 'D', submit: 'S', cancel: 'X', amend: 'A', audit: 'V' }

export function actionLetter(action: string): string {
  return actionLetters[action] ?? '?'
}

export const actionLegend: [string, string][] = [
  ['R', 'read'],
  ['C', 'create'],
  ['W', 'write'],
  ['D', 'delete'],
  ['S', 'submit'],
  ['X', 'cancel'],
  ['A', 'amend'],
  ['V', 'audit'],
]

export type GrantLike = { recordTypeKey: string; actions: string[]; permissionLevel: number; condition: string }

// previewRoleGrants summarizes what a Role allows, one line per Grant, for
// the assignment preview. Labels name Record Types for display.
export function previewRoleGrants(grants: GrantLike[], labels: Record<string, string>, scope: ScopeMap): string[] {
  const title = (text: string) => text.slice(0, 1).toUpperCase() + text.slice(1)
  return grants.map((grant) => {
    const actions = grant.actions.map(title).join(', ') || 'Nothing'
    const target = grant.recordTypeKey === '*' ? 'everything' : (labels[grant.recordTypeKey] ?? grant.recordTypeKey)
    let line = `${actions} ${target}`
    const described = describeScope(scope)
    if (described !== 'Everywhere') line += ` in ${described}`
    if (grant.permissionLevel > 0) line += ` (field level ${grant.permissionLevel})`
    if (grant.condition) line += ` where ${grant.condition}`
    return line + '.'
  })
}
