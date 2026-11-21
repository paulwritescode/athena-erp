import { describe, expect, it } from 'vitest'
import { create } from '@bufbuild/protobuf'
import { FormLayoutSchema } from '../gen/athena/v1/record_types_pb'
import { layoutFromMessage, layoutToMessage, planSections, sameLayout } from './layout'
import type { Field } from '../types'

const name: Field = { name: 'name', label: 'Name', type: 'Text', required: true }
const notes: Field = { name: 'notes', label: 'Notes', type: 'Long text', required: false }
const photo: Field = { name: 'photo', label: 'Photo', type: 'Image', required: false }

function wire() {
  return create(FormLayoutSchema, {
    sections: [
      {
        title: 'Basics', description: 'Who this is.', columns: 2, visibility: 'doc.name != ""',
        items: [
          { kind: { case: 'field', value: { fieldKey: 'name', width: 'half', helpText: 'The full name.' } } },
          { kind: { case: 'text', value: { text: 'Names are unique.' } } },
        ],
      },
      {
        title: 'More', description: '', columns: 1, visibility: '',
        items: [
          { kind: { case: 'image', value: { path: 'images/banner.png' } } },
          { kind: { case: 'art', value: { key: 'stack' } } },
        ],
      },
    ],
  })
}

describe('form layout round trip', () => {
  it('reads every section, item kind and attribute off the wire', () => {
    const layout = layoutFromMessage(wire())
    expect(layout?.sections).toHaveLength(2)
    expect(layout?.sections[0]).toMatchObject({ title: 'Basics', description: 'Who this is.', columns: 2, visibility: 'doc.name != ""' })
    expect(layout?.sections[0]?.items).toEqual([
      { kind: 'field', fieldKey: 'name', width: 'half', helpText: 'The full name.' },
      { kind: 'text', text: 'Names are unique.' },
    ])
    expect(layout?.sections[1]?.items).toEqual([
      { kind: 'image', path: 'images/banner.png' },
      { kind: 'art', key: 'stack' },
    ])
  })

  it('writes back exactly what it read', () => {
    const message = wire()
    expect(sameLayout(layoutFromMessage(message), layoutFromMessage(message))).toBe(true)
    const roundTripped = layoutToMessage(layoutFromMessage(message))
    expect(roundTripped).toEqual(message)
  })

  it('treats absent and empty layouts as no layout', () => {
    expect(layoutFromMessage(undefined)).toBeUndefined()
    expect(layoutFromMessage(create(FormLayoutSchema, { sections: [] }))).toBeUndefined()
    expect(layoutToMessage(undefined)).toBeUndefined()
    expect(layoutToMessage({ sections: [] })).toBeUndefined()
    expect(sameLayout(undefined, { sections: [] })).toBe(true)
    expect(sameLayout(layoutFromMessage(wire()), undefined)).toBe(false)
  })

  it('repairs an unknown width to full instead of dropping the item', () => {
    const message = create(FormLayoutSchema, {
      sections: [{ title: '', description: '', columns: 1, visibility: '', items: [{ kind: { case: 'field', value: { fieldKey: 'name', width: 'third', helpText: '' } } }] }],
    })
    expect(layoutFromMessage(message)?.sections[0]?.items).toEqual([{ kind: 'field', fieldKey: 'name', width: 'full', helpText: '' }])
  })
})

describe('planSections', () => {
  it('renders fields in definition order in one column without a layout', () => {
    const planned = planSections(undefined, [name, notes])
    expect(planned).toHaveLength(1)
    expect(planned[0]?.columns).toBe(1)
    expect(planned[0]?.items).toEqual([
      { kind: 'field', field: name, width: 'full', helpText: '' },
      { kind: 'field', field: notes, width: 'full', helpText: '' },
    ])
  })

  it('appends unplaced fields to the last section at full width', () => {
    const planned = planSections(layoutFromMessage(wire()), [name, notes, photo])
    expect(planned).toHaveLength(2)
    // The first section keeps its order: the field, then the text block.
    expect(planned[0]?.items.map((item) => item.kind)).toEqual(['field', 'text'])
    const last = planned[1]?.items ?? []
    expect(last.map((item) => item.kind)).toEqual(['image', 'art', 'field', 'field'])
    expect(last[2]).toEqual({ kind: 'field', field: notes, width: 'full', helpText: '' })
    expect(last[3]).toEqual({ kind: 'field', field: photo, width: 'full', helpText: '' })
  })

  it('drops items that name no field instead of rendering them', () => {
    const layout = layoutFromMessage(create(FormLayoutSchema, {
      sections: [{ title: 'S', description: '', columns: 1, visibility: '', items: [{ kind: { case: 'field', value: { fieldKey: 'gone', width: '', helpText: '' } } }] }],
    }))
    const planned = planSections(layout, [name])
    expect(planned[0]?.items).toEqual([{ kind: 'field', field: name, width: 'full', helpText: '' }])
  })

  it('renders anything but two columns as one', () => {
    const layout = layoutFromMessage(create(FormLayoutSchema, {
      sections: [{ title: 'S', description: '', columns: 7, visibility: '', items: [] }],
    }))
    expect(planSections(layout, [name])[0]?.columns).toBe(1)
  })
})
