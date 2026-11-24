// The Rule definition rules the server applies in checkDefinition
// (internal/server/rules.go) and internal/rules. They are repeated here only
// so a mistake shows before the request; the server stays the authority.
import { compileTree, type ConditionTree, type TreeField } from './conditions'

export const ruleKeyPattern = /^[a-z][a-z0-9_]{0,62}$/
export const maxRulePriority = 1000
export const maxRuleSourceBytes = 64 * 1024
export const maxRuleMessageLength = 500

export type RuleAuthoring = { mode: 'builder' | 'starlark'; tree: ConditionTree | null; message: string; sets: Array<{ field: string; valueExpr: string }>; fields: Record<string, TreeField> }

export const ruleTypes = ['validation', 'derivation', 'guard', 'transform'] as const
export type RuleType = (typeof ruleTypes)[number]

// Lifecycle events a Rule of each type may attach to. A transform has none.
export const ruleEvents: Record<RuleType, string[]> = {
  validation: ['validate', 'before_submit'],
  derivation: ['validate'],
  guard: ['before_submit', 'before_cancel', 'on_trash'],
  transform: [],
}

export const ruleTypeHelp: Record<RuleType, string> = {
  validation: 'Rejects a save or submit with require(), fail() or warn(). Cannot change data.',
  derivation: 'Sets fields of the document with set(). Runs before validations.',
  guard: 'Allows or denies a transition. Cannot change data.',
  transform: 'Reshapes values when called, for example from Simulate. Reads and writes nothing.',
}

export const starterSource: Record<RuleType, string> = {
  validation: 'def validate(doc, ctx):\n    require(True, "Explain what must hold")\n',
  derivation: 'def derive(doc, ctx):\n    pass\n',
  guard: 'def guard(doc, ctx):\n    require(True, "Explain why the transition is blocked")\n',
  transform: 'def transform(row, ctx):\n    return {}\n',
}

export function isRuleType(value: string): value is RuleType {
  return (ruleTypes as readonly string[]).includes(value)
}

// Returns the first problem with a definition, or null. Builder Rules pass
// their authoring instead of source: the tree must compile, validation and
// guard need a message, derivation needs at least one set field.
export function ruleProblem(def: { key: string; label: string; ruleType: string; recordTypeKey: string; event: string; priority: number; source: string }, authoring?: RuleAuthoring): string | null {
  if (!ruleKeyPattern.test(def.key)) return 'The key starts with a lowercase letter and holds lowercase letters, digits and underscores, up to 63 characters.'
  if (!def.label.trim() || [...def.label].length > 255) return 'Give the Rule a label of up to 255 characters.'
  if (!isRuleType(def.ruleType)) return 'Pick a Rule type.'
  if (def.ruleType !== 'transform' && !def.recordTypeKey) return 'Pick the Record Type the Rule belongs to.'
  if (def.ruleType === 'transform' ? def.event !== '' : !ruleEvents[def.ruleType].includes(def.event)) return 'Pick an event this Rule type can run at.'
  if (!Number.isInteger(def.priority) || def.priority < 0 || def.priority > maxRulePriority) return 'The priority is a whole number from 0 to 1000.'
  if (authoring?.mode === 'builder') {
    if (!authoring.tree) return 'Build the condition the Rule checks.'
    try {
      compileTree(authoring.tree, authoring.fields)
    } catch (err) {
      return err instanceof Error ? err.message : String(err)
    }
    if ((def.ruleType === 'validation' || def.ruleType === 'guard') && !authoring.message.trim()) {
      return def.ruleType === 'validation' ? 'Say what the save shows when the check fails.' : 'Say why the transition is blocked.'
    }
    if ([...authoring.message].length > maxRuleMessageLength) return 'The message holds up to 500 characters.'
    if (def.ruleType === 'derivation') {
      if (authoring.sets.length === 0) return 'Set at least one field.'
      for (const set of authoring.sets) {
        if (!set.field || !set.valueExpr.trim()) return 'Every set field needs a field and a value.'
      }
    }
    return null
  }
  if (!def.source.trim()) return 'The Rule has no source.'
  if (new TextEncoder().encode(def.source).length > maxRuleSourceBytes) return 'The source is over 64 KiB.'
  return null
}
