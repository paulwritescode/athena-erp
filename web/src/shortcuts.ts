// The one keyboard-shortcut registry. Shortcuts are written as key names
// joined with '+', case-insensitive: 'cmd+k', 'ctrl+shift+p', 'escape'.
// Modifiers accept cmd/command/⌘/meta/ctrl/control/alt/option/shift; keys
// accept letters, digits and enter/return/space/tab/esc/escape/delete/
// backspace/up/down/left/right/home/end. Leading modifier symbols peel off,
// so display strings ('⌘K') register as well as joined ones ('cmd+k'). One
// window listener dispatches to the registered owner; registering a taken
// combo warns and keeps the first owner, so conflicts surface instead of
// double-firing. Escape stays reserved for dismissal: dialogs and menus own
// it, actions must not take it.
export type ShortcutHandler = () => void

type Parsed = { key: string; meta: boolean; ctrl: boolean; alt: boolean; shift: boolean }

const MODIFIERS: Record<string, 'meta' | 'ctrl' | 'alt' | 'shift'> = {
  cmd: 'meta',
  command: 'meta',
  '⌘': 'meta',
  meta: 'meta',
  ctrl: 'ctrl',
  control: 'ctrl',
  alt: 'alt',
  option: 'alt',
  shift: 'shift',
}

const KEY_ALIASES: Record<string, string> = {
  return: 'enter',
  esc: 'escape',
  del: 'delete',
  backspace: 'delete',
  up: 'arrowup',
  down: 'arrowdown',
  left: 'arrowleft',
  right: 'arrowright',
  ' ': 'space',
}

const LEADING_SYMBOLS: Record<string, 'meta' | 'ctrl' | 'alt' | 'shift'> = {
  '⌘': 'meta',
  '⌃': 'ctrl',
  '⌥': 'alt',
  '⇧': 'shift',
}

export function parseShortcut(combo: string): Parsed | null {
  const parts = combo
    .split('+')
    .map((part) => part.trim().toLowerCase())
    .filter((part) => part.length > 0)
  if (parts.length === 0) return null
  const parsed: Parsed = { key: '', meta: false, ctrl: false, alt: false, shift: false }
  const take = (part: string): boolean => {
    let rest = part
    while (rest.length > 0 && LEADING_SYMBOLS[rest[0]]) {
      parsed[LEADING_SYMBOLS[rest[0]]] = true
      rest = rest.slice(1)
    }
    if (rest === '') return true
    const modifier = MODIFIERS[rest]
    if (modifier) {
      parsed[modifier] = true
      return true
    }
    if (parsed.key !== '') return false
    parsed.key = KEY_ALIASES[rest] ?? rest
    return true
  }
  for (const part of parts) {
    if (!take(part)) return null
  }
  if (parsed.key === '') return null
  return parsed
}

export function shortcutId(parsed: Parsed): string {
  return [
    parsed.meta ? 'meta' : '',
    parsed.ctrl ? 'ctrl' : '',
    parsed.alt ? 'alt' : '',
    parsed.shift ? 'shift' : '',
    parsed.key,
  ]
    .filter((part) => part.length > 0)
    .join('+')
}

export function eventMatches(event: { key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }, parsed: Parsed): boolean {
  const key = event.key.toLowerCase() === ' ' ? 'space' : event.key.toLowerCase()
  return key === parsed.key && event.metaKey === parsed.meta && event.ctrlKey === parsed.ctrl && event.altKey === parsed.alt && event.shiftKey === parsed.shift
}

const owners = new Map<string, { owner: string; handler: ShortcutHandler; combo: Parsed }>()
const byHandler = new Map<ShortcutHandler, string>()
let listening = false

function onKeyDown(event: KeyboardEvent) {
  const target = event.target as HTMLElement | null
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return
  for (const entry of owners.values()) {
    if (eventMatches(event, entry.combo)) {
      event.preventDefault()
      entry.handler()
      return
    }
  }
}

function ensureListening() {
  if (listening || typeof window === 'undefined') return
  window.addEventListener('keydown', onKeyDown)
  listening = true
}

// Registers combo for owner. Returns true, or false when the combo is taken
// or invalid (and warns, so the conflict is visible in development).
export function registerShortcut(combo: string, owner: string, handler: ShortcutHandler): boolean {
  const parsed = parseShortcut(combo)
  if (!parsed) {
    console.warn(`[shortcuts] ignoring invalid combo "${combo}" for ${owner}`)
    return false
  }
  if (parsed.key === 'escape' && !parsed.meta && !parsed.ctrl && !parsed.alt && !parsed.shift) {
    console.warn(`[shortcuts] bare Escape is reserved for dismissal; ${owner} cannot take it`)
    return false
  }
  const id = shortcutId(parsed)
  const taken = owners.get(id)
  if (taken) {
    console.warn(`[shortcuts] "${combo}" for ${owner} conflicts with ${taken.owner}; keeping ${taken.owner}`)
    return false
  }
  owners.set(id, { owner, handler, combo: parsed })
  byHandler.set(handler, id)
  ensureListening()
  return true
}

export function unregisterShortcut(handler: ShortcutHandler): void {
  const id = byHandler.get(handler)
  if (!id) return
  if (owners.get(id)?.handler === handler) owners.delete(id)
  byHandler.delete(handler)
}

export function shortcutOwners(): { combo: string; owner: string }[] {
  return [...owners.entries()].map(([combo, entry]) => ({ combo, owner: entry.owner }))
}
