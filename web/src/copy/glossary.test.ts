import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { gloss, glossary, glossaryKeys, type GlossaryKey } from './glossary'

const root = dirname(fileURLToPath(import.meta.url))

function files(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) {
      if (entry === 'gen') continue
      files(path, out)
    } else if (entry.endsWith('.vue') || entry.endsWith('.ts')) {
      out.push(path)
    }
  }
  return out
}

// Comments explain code to builders, so they may name builder words; the
// check below only reads what a user can see.
function stripComments(text: string): string {
  return text
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|\s)\/\/.*$/gm, '$1')
}

describe('glossary', () => {
  it('gives every term a label and a one-line explanation', () => {
    const keys = glossaryKeys()
    expect(keys.length).toBeGreaterThan(0)
    for (const key of keys) {
      const entry = gloss(key)
      expect(entry.label.trim(), `${key} label`).not.toBe('')
      expect(entry.explanation.trim(), `${key} explanation`).not.toBe('')
      expect(entry.explanation, `${key} one line`).not.toMatch(/\n/)
      expect(entry.explanation, `${key} sentence`).toMatch(/^[A-Z].*\.$/)
    }
  })

  it('covers every area the B6 brief names', () => {
    const keys = new Set<string>(glossaryKeys())
    const required: GlossaryKey[] = [
      // Lifecycle.
      'draft', 'submitted', 'cancelled', 'amended',
      // Record kinds (playbook section 1: nine kinds).
      'kindMaster', 'kindDocument', 'kindChild', 'kindLedger', 'kindBalance',
      'kindSetting', 'kindLog', 'kindSnapshot', 'kindMeta',
      // Ledger.
      'company', 'financialYear', 'period', 'softClose', 'hardClose', 'currency',
      'exchangeRate', 'account', 'accountType', 'posting',
      // Access.
      'role', 'grant', 'assignment', 'scope', 'session', 'stepUp', 'invite', 'owner', 'rootAdministrator',
      // Rules.
      'rule', 'validation', 'derivation', 'guard',
      // Packs and workspaces.
      'pack', 'workspace',
      // Notification kinds (internal/notifications/inbox.go).
      'inviteAccepted', 'accessChanged', 'signInFailed', 'accountLocked',
      'sessionEnded', 'saveStopped', 'periodClosed',
    ]
    for (const key of required) {
      expect(keys, `glossary covers ${key}`).toContain(key)
    }
  })

  it('defines every key the desk uses', () => {
    const src = join(root, '..')
    const known = new Set<string>(glossaryKeys())
    const used = new Map<string, string>()
    const collect = (text: string, file: string, pattern: RegExp) => {
      for (const match of text.matchAll(pattern)) {
        if (match[1] && !used.has(match[1])) used.set(match[1], file)
      }
    }
    for (const file of files(src)) {
      if (file.endsWith('glossary.ts') || file.endsWith('glossary.test.ts')) continue
      const text = readFileSync(file, 'utf8')
      collect(text, file, /(?<!:)\bterm="([A-Za-z]+)"/g)
      collect(text, file, /:term="'([A-Za-z]+)'"/g)
      collect(text, file, /\bgloss\("([A-Za-z]+)"\)/g)
      collect(text, file, /\bgloss\('([A-Za-z]+)'\)/g)
      for (const hints of text.matchAll(/:hints="\[([^\]]*)\]"/g) ) {
        for (const key of (hints[1] ?? '').matchAll(/'([A-Za-z]+)'/g)) {
          if (key[1] && !used.has(key[1])) used.set(key[1], file)
        }
      }
    }
    expect(used.size, 'desk uses glossary keys').toBeGreaterThan(0)
    for (const [key, file] of used) {
      expect(known, `${key} used in ${file}`).toContain(key)
    }
  })

  it('keeps builder-only words out of View-mode files', () => {
    const viewFiles = [
      'pages/RecordsPage.vue',
      'pages/WorkQueuePage.vue',
      'pages/CountPage.vue',
      'components/records/recordColumns.ts',
      'components/shell/InboxBody.vue',
    ]
    const builderOnly = [/\bRecord Types?\b/, /\bPacks?\b/, /\bRules?\b/, /\bGrants?\b/]
    for (const relative of viewFiles) {
      const visible = stripComments(readFileSync(join(root, '..', relative), 'utf8'))
      for (const pattern of builderOnly) {
        expect(visible, `${relative} has no ${pattern}`).not.toMatch(pattern)
      }
    }
  })

  it('keeps the shared record fields free of builder-only words', () => {
    const builderOnly = [/\bRecord Types?\b/, /\bPacks?\b/, /\bRules?\b/, /\bGrants?\b/]
    for (const file of files(join(root, '..', 'components', 'fields'))) {
      const visible = stripComments(readFileSync(file, 'utf8'))
      for (const pattern of builderOnly) {
        expect(visible, `${file} has no ${pattern}`).not.toMatch(pattern)
      }
    }
  })

  it('exports a typed lookup', () => {
    expect(gloss('softClose')).toEqual(glossary.softClose)
    const key: GlossaryKey = 'hardClose'
    expect(gloss(key).label).toBe('Hard close')
  })
})
