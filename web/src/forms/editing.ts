// Pure edits for the form builder (B9 sub-slice 2). Each function mutates the
// draft layout in place — the editor saves it through the existing save path
// — and reports where the change landed so the builder can announce it and
// move focus. Bounds the server enforces (50 sections, 500 items) are left
// for the server; the builder never blocks typing on them.
import type { Field } from '../types'
import type { FormLayout, LayoutItem, LayoutSection } from './layout'

/** The keys placed anywhere in the layout. */
export function placedKeys(layout: FormLayout | undefined): Set<string> {
  const placed = new Set<string>()
  for (const section of layout?.sections ?? []) {
    for (const item of section.items) {
      if (item.kind === 'field') placed.add(item.fieldKey)
    }
  }
  return placed
}

/** Fields the layout does not place yet, in definition order. */
export function unplacedFields(layout: FormLayout | undefined, fields: Field[]): Field[] {
  const placed = placedKeys(layout)
  return fields.filter((field) => !placed.has(field.name))
}

export function blankSection(title = ''): LayoutSection {
  return { title, description: '', columns: 1, visibility: '', items: [] }
}

/** Starts the layout from the current fields: one section, everything full width. */
export function arrangeAll(layout: FormLayout, fields: Field[]): void {
  layout.sections = [{ ...blankSection(), items: fields.map((field): LayoutItem => ({ kind: 'field', fieldKey: field.name, width: 'full', helpText: '' })) }]
}

/** Appends a section; returns its index. */
export function addSection(layout: FormLayout, title = ''): number {
  layout.sections.push(blankSection(title))
  return layout.sections.length - 1
}

/** Deletes a section; its fields become unplaced. Returns the section title for the announcement. */
export function deleteSection(layout: FormLayout, index: number): string {
  const [removed] = layout.sections.splice(index, 1)
  return removed?.title ?? ''
}

/** Moves a section up (-1) or down (+1); returns the new index, or -1 at the edge. */
export function moveSection(layout: FormLayout, index: number, direction: -1 | 1): number {
  const next = index + direction
  if (next < 0 || next >= layout.sections.length) return -1
  const sections = layout.sections
  ;[sections[index], sections[next]] = [sections[next] as LayoutSection, sections[index] as LayoutSection]
  return next
}

/** Appends an item; returns its index in the section. */
export function addItem(layout: FormLayout, sectionIndex: number, item: LayoutItem): number {
  const section = layout.sections[sectionIndex]
  if (!section) return -1
  section.items.push(item)
  return section.items.length - 1
}

/** Removes an item; a field item becomes unplaced. */
export function removeItem(layout: FormLayout, sectionIndex: number, itemIndex: number): void {
  layout.sections[sectionIndex]?.items.splice(itemIndex, 1)
}

/** Where a moved item landed. */
export type ItemPosition = { section: number; index: number }

/**
 * Moves an item within its section (-1 up, +1 down); returns the new index,
 * or -1 at the edge. Moving across sections is moveItemTo.
 */
export function moveItem(layout: FormLayout, sectionIndex: number, itemIndex: number, direction: -1 | 1): number {
  const items = layout.sections[sectionIndex]?.items
  if (!items) return -1
  const next = itemIndex + direction
  if (next < 0 || next >= items.length) return -1
  ;[items[itemIndex], items[next]] = [items[next] as LayoutItem, items[itemIndex] as LayoutItem]
  return next
}

/** Moves an item to any position in any section; returns where it landed. */
export function moveItemTo(layout: FormLayout, fromSection: number, fromIndex: number, toSection: number, toIndex: number): ItemPosition {
  const from = layout.sections[fromSection]?.items
  const to = layout.sections[toSection]?.items
  const item = from?.[fromIndex]
  if (!from || !to || !item) return { section: fromSection, index: fromIndex }
  from.splice(fromIndex, 1)
  let at = toIndex
  if (to === from && at > fromIndex) at--
  at = Math.max(0, Math.min(at, to.length))
  to.splice(at, 0, item)
  return { section: toSection, index: at }
}
