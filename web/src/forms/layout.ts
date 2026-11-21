// The form layout of a Record Type (B9, ADR-0027): sections of fields, static
// text and image blocks, and art headers. The server stores the layout with
// the definition and appends fields the layout does not place to the last
// section; planSections repeats that here so the desk also renders an old or
// hand-written layout completely.
import { create, equals } from '@bufbuild/protobuf'
import { FormLayoutSchema, type FormLayout as FormLayoutMessage } from '../gen/athena/v1/record_types_pb'
import type { Field } from '../types'

export type LayoutWidth = 'full' | 'half' | ''

/** True for the widths the server accepts (internal/server/layout.go). */
export function isLayoutWidth(width: string): width is LayoutWidth {
  return width === '' || width === 'full' || width === 'half'
}

export type LayoutFieldItem = { kind: 'field'; fieldKey: string; width: LayoutWidth; helpText: string }
export type LayoutTextItem = { kind: 'text'; text: string }
export type LayoutImageItem = { kind: 'image'; path: string }
export type LayoutArtItem = { kind: 'art'; key: string }
export type LayoutItem = LayoutFieldItem | LayoutTextItem | LayoutImageItem | LayoutArtItem

export type LayoutSection = { title: string; description: string; columns: number; visibility: string; items: LayoutItem[] }
export type FormLayout = { sections: LayoutSection[] }

/** A section ready to render: field items resolved, unknown keys dropped. */
export type PlannedFieldItem = { kind: 'field'; field: Field; width: LayoutWidth; helpText: string }
export type PlannedItem = PlannedFieldItem | LayoutTextItem | LayoutImageItem | LayoutArtItem
export type PlannedSection = { title: string; description: string; columns: 1 | 2; visibility: string; items: PlannedItem[] }

/** Reads a layout off the wire. Absent or empty reads as no layout. */
export function layoutFromMessage(message: FormLayoutMessage | undefined): FormLayout | undefined {
  if (!message || message.sections.length === 0) return undefined
  return {
    sections: message.sections.map((section) => ({
      title: section.title,
      description: section.description,
      columns: section.columns,
      visibility: section.visibility,
      items: section.items.flatMap((item): LayoutItem[] => {
        switch (item.kind.case) {
          case 'field':
            return [{ kind: 'field', fieldKey: item.kind.value.fieldKey, width: isLayoutWidth(item.kind.value.width) ? item.kind.value.width : 'full', helpText: item.kind.value.helpText }]
          case 'text':
            return [{ kind: 'text', text: item.kind.value.text }]
          case 'image':
            return [{ kind: 'image', path: item.kind.value.path }]
          case 'art':
            return [{ kind: 'art', key: item.kind.value.key }]
          default:
            return []
        }
      }),
    })),
  }
}

/** Writes a layout for the wire. Absent or empty writes as no layout. */
export function layoutToMessage(layout: FormLayout | undefined): FormLayoutMessage | undefined {
  if (!layout || layout.sections.length === 0) return undefined
  return create(FormLayoutSchema, {
    sections: layout.sections.map((section) => ({
      title: section.title,
      description: section.description,
      columns: section.columns,
      visibility: section.visibility,
      items: section.items.map((item) => {
        switch (item.kind) {
          case 'field':
            return { kind: { case: 'field' as const, value: { fieldKey: item.fieldKey, width: item.width, helpText: item.helpText } } }
          case 'text':
            return { kind: { case: 'text' as const, value: { text: item.text } } }
          case 'image':
            return { kind: { case: 'image' as const, value: { path: item.path } } }
          case 'art':
            return { kind: { case: 'art' as const, value: { key: item.key } } }
        }
      }),
    })),
  })
}

/** True when two layouts carry the same sections and items. */
export function sameLayout(a: FormLayout | undefined, b: FormLayout | undefined): boolean {
  const x = layoutToMessage(a)
  const y = layoutToMessage(b)
  if (!x || !y) return !x && !y
  return equals(FormLayoutSchema, x, y)
}

/**
 * Plans what the form renders. Without a layout every field renders in
 * definition order in one column (R9.5). With one, items keep their order,
 * field items resolve to their field (unknown keys are dropped, never
 * rendered), and fields the layout does not place are appended to the last
 * section at full width, as the server does on save.
 */
export function planSections(layout: FormLayout | undefined, fields: Field[]): PlannedSection[] {
  if (!layout || layout.sections.length === 0) {
    return [{ title: '', description: '', columns: 1, visibility: '', items: fields.map((field): PlannedItem => ({ kind: 'field', field, width: 'full', helpText: '' })) }]
  }
  const byKey = new Map(fields.map((field) => [field.name, field]))
  const placed = new Set<string>()
  const sections: PlannedSection[] = layout.sections.map((section) => ({
    title: section.title,
    description: section.description,
    columns: section.columns === 2 ? 2 : 1,
    visibility: section.visibility,
    items: section.items.flatMap((item): PlannedItem[] => {
      if (item.kind !== 'field') return [item]
      const field = byKey.get(item.fieldKey)
      if (!field) return []
      placed.add(field.name)
      return [{ kind: 'field', field, width: item.width, helpText: item.helpText }]
    }),
  }))
  const last = sections[sections.length - 1]
  if (last) {
    for (const field of fields) {
      if (!placed.has(field.name)) last.items.push({ kind: 'field', field, width: 'full', helpText: '' })
    }
  }
  return sections
}
