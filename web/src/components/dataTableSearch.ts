// The shared table search box (F2): one input whose placeholder derives from
// the rows it searches ("Diet concern" becomes "Find diet concerns…") and
// whose keystrokes settle before the table searches, so one burst of typing
// filters once. F4's Advanced filter builds on this block.
import { gloss } from '@/copy/glossary'

/** Keystrokes settle this long before the table searches. */
export const SEARCH_DEBOUNCE_MS = 150

/**
 * Lowercase English plural of a scope label: "Diet concern" becomes
 * "diet concerns", "Branch" becomes "branches", "Company" becomes
 * "companies". The desk is English-only, so no locale rules apply.
 */
export function pluralScope(label: string): string {
  const words = label.trim().toLowerCase()
  if (words === '') return words
  if (words.endsWith('y') && !/[aeiou]y$/.test(words)) return `${words.slice(0, -1)}ies`
  if (/(s|x|z|ch|sh)$/.test(words)) return `${words}es`
  return `${words}s`
}

/** "Find diet concerns…" from a Record Type label. */
export function searchPlaceholderFor(label: string): string {
  return `${gloss('find').label} ${pluralScope(label)}…`
}
