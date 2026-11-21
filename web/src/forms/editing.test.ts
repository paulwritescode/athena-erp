import { describe, expect, it } from 'vitest'
import { addItem, addSection, arrangeAll, blankSection, deleteSection, moveItem, moveItemTo, moveSection, placedKeys, removeItem, unplacedFields } from './editing'
import type { FormLayout } from './layout'
import type { Field } from '../types'

const name: Field = { name: 'name', label: 'Name', type: 'Text', required: true }
const price: Field = { name: 'price', label: 'Price', type: 'Number', required: false }
const photo: Field = { name: 'photo', label: 'Photo', type: 'Image', required: false }

function layout(): FormLayout {
  return {
    sections: [
      { ...blankSection('Basics'), columns: 2, items: [{ kind: 'field', fieldKey: 'name', width: 'half', helpText: '' }] },
      { ...blankSection('More'), items: [{ kind: 'field', fieldKey: 'price', width: 'full', helpText: '' }] },
    ],
  }
}

describe('form builder edits', () => {
  it('arranges every field into one full-width section', () => {
    const draft: FormLayout = { sections: [] }
    arrangeAll(draft, [name, price])
    expect(draft.sections).toHaveLength(1)
    expect(draft.sections[0]?.items).toEqual([
      { kind: 'field', fieldKey: 'name', width: 'full', helpText: '' },
      { kind: 'field', fieldKey: 'price', width: 'full', helpText: '' },
    ])
  })

  it('tracks placed and unplaced fields', () => {
    const draft = layout()
    expect(placedKeys(draft)).toEqual(new Set(['name', 'price']))
    expect(unplacedFields(draft, [name, price, photo])).toEqual([photo])
    removeItem(draft, 0, 0)
    expect(unplacedFields(draft, [name, price, photo]).map((field) => field.name)).toEqual(['name', 'photo'])
  })

  it('adds and deletes sections, freeing their fields', () => {
    const draft = layout()
    const at = addSection(draft, 'Extra')
    expect(draft.sections).toHaveLength(3)
    expect(draft.sections[at]?.title).toBe('Extra')
    expect(deleteSection(draft, 0)).toBe('Basics')
    expect(unplacedFields(draft, [name, price, photo]).map((field) => field.name)).toEqual(['name', 'photo'])
  })

  it('moves sections up and down, stopping at the edges', () => {
    const draft = layout()
    expect(moveSection(draft, 0, -1)).toBe(-1)
    expect(moveSection(draft, 1, 1)).toBe(-1)
    expect(moveSection(draft, 0, 1)).toBe(1)
    expect(draft.sections.map((section) => section.title)).toEqual(['More', 'Basics'])
  })

  it('adds items at the end of a section', () => {
    const draft = layout()
    expect(addItem(draft, 1, { kind: 'text', text: 'Hello' })).toBe(1)
    expect(addItem(draft, 9, { kind: 'text', text: 'Lost' })).toBe(-1)
    expect(draft.sections[1]?.items.map((item) => item.kind)).toEqual(['field', 'text'])
  })

  it('moves items up and down within a section', () => {
    const draft = layout()
    addItem(draft, 0, { kind: 'text', text: 'Hello' })
    expect(moveItem(draft, 0, 0, -1)).toBe(-1)
    expect(moveItem(draft, 0, 0, 1)).toBe(1)
    expect(draft.sections[0]?.items.map((item) => item.kind)).toEqual(['text', 'field'])
    expect(moveItem(draft, 0, 1, 1)).toBe(-1)
  })

  it('moves items across sections and within one by index', () => {
    const draft = layout()
    const landed = moveItemTo(draft, 0, 0, 1, 0)
    expect(landed).toEqual({ section: 1, index: 0 })
    expect(draft.sections[0]?.items).toEqual([])
    expect(draft.sections[1]?.items.map((item) => (item.kind === 'field' ? item.fieldKey : item.kind))).toEqual(['name', 'price'])
    // Past the end clamps to the end.
    expect(moveItemTo(draft, 1, 0, 1, 99)).toEqual({ section: 1, index: 1 })
    // Unknown positions stay put.
    expect(moveItemTo(draft, 7, 0, 1, 0)).toEqual({ section: 7, index: 0 })
  })
})
