// Shared action model for ActionButton and ActionGroup, kept pure so the
// grouping math is unit-testable without a DOM. Kinds follow design-system
// section 14; the caller owns the order (destructive last, restore where its
// opposite sits) and the group only breaks the row where the kind changes.
import type { IconName } from '@/icons'

export type ActionKind = 'primary' | 'secondary' | 'quiet' | 'caution' | 'destructive' | 'restore'

export type ActionItem = {
  id: string
  label: string
  kind?: ActionKind
  icon?: IconName
  iconOnly?: boolean
  counter?: number | string | null
  shortcut?: string
  helper?: string
  busy?: boolean
  disabledReason?: string | null
}

export function actionKind(action: ActionItem): ActionKind {
  return action.kind ?? 'secondary'
}

// chunkByKind groups consecutive actions of the same kind, so the group can
// space within a kind (--space-2) differently from between kinds (--space-4).
// A chunk whose kind is destructive and which is not first renders behind a
// hairline divider.
export type ActionChunk = { kind: ActionKind; actions: ActionItem[]; dividerBefore: boolean }

export function chunkByKind(actions: ActionItem[]): ActionChunk[] {
  const chunks: ActionChunk[] = []
  for (const action of actions) {
    const kind = actionKind(action)
    const last = chunks[chunks.length - 1]
    if (last && last.kind === kind) last.actions.push(action)
    else chunks.push({ kind, actions: [action], dividerBefore: kind === 'destructive' && chunks.length > 0 })
  }
  return chunks
}
