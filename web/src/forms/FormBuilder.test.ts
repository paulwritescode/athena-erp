// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, nextTick, reactive, type Component } from 'vue'
import FormBuilder from './FormBuilder.vue'
import type { FormLayout } from './layout'
import type { RecordType } from '../types'

const mounted: { unmount: () => void; el: HTMLElement }[] = []

afterEach(() => {
  for (const m of mounted.splice(0)) {
    m.unmount()
    m.el.remove()
  }
  document.body.innerHTML = ''
})

function mount(recordType: RecordType): HTMLElement {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(FormBuilder as Component, { recordType })
  app.mount(el)
  mounted.push({ unmount: () => app.unmount(), el })
  return el
}

function item(): RecordType {
  return {
    name: 'item', label: 'Item', description: '', kind: 'master', revision: 1,
    fields: [
      { name: 'name', label: 'Name', type: 'Text', required: true },
      { name: 'price', label: 'Price', type: 'Number', required: false },
      { name: 'notes', label: 'Notes', type: 'Long text', required: false },
    ],
    layout: {
      sections: [
        {
          title: 'Basics', description: '', columns: 2, visibility: '',
          items: [
            { kind: 'field', fieldKey: 'name', width: 'half', helpText: '' },
            { kind: 'field', fieldKey: 'price', width: 'half', helpText: '' },
          ],
        },
        { title: 'More', description: '', columns: 1, visibility: '', items: [] },
      ],
    },
  }
}

function keys(recordType: RecordType, section: number): (string | undefined)[] {
  return (recordType.layout?.sections[section]?.items ?? []).map((entry) => (entry.kind === 'field' ? entry.fieldKey : entry.kind))
}

function announced(el: HTMLElement): string {
  return el.querySelector('[role="status"]')?.textContent ?? ''
}

// Announcements and focus moves queue behind the click's own update.
async function settle() {
  await nextTick()
  await nextTick()
  await nextTick()
}

describe('form builder keyboard reorder', () => {
  it('moves an item down and up with the buttons, announcing each move', async () => {
    const recordType = reactive(item())
    const el = mount(recordType)
    ;(el.querySelector('[data-row="0:0"] [data-action="down"]') as HTMLButtonElement).click()
    await settle()
    expect(keys(recordType, 0)).toEqual(['price', 'name'])
    expect(announced(el)).toBe('Name moved down.')
    // Focus follows the moved row, so repeated presses keep working.
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Move Name up')
    ;(el.querySelector('[data-row="0:1"] [data-action="up"]') as HTMLButtonElement).click()
    await settle()
    expect(keys(recordType, 0)).toEqual(['name', 'price'])
    expect(announced(el)).toBe('Name moved up.')
  })

  it('disables move buttons at the edges with a reason', async () => {
    const recordType = reactive(item())
    const el = mount(recordType)
    await nextTick()
    const up = el.querySelector('[data-row="0:0"] [data-action="up"]') as HTMLButtonElement
    expect(up.disabled).toBe(true)
    expect(up.getAttribute('aria-label')).toContain('Already at the top')
  })

  it('moves an item to another section from the menu', async () => {
    const recordType = reactive(item())
    const el = mount(recordType)
    ;(el.querySelector('[data-row="0:1"] [data-action="move"]') as HTMLButtonElement).click()
    await nextTick()
    await nextTick()
    const options = [...document.body.querySelectorAll('[role="menuitem"]')]
    const more = options.find((option) => option.textContent?.includes('More'))
    expect(more, 'move menu lists the other section').toBeTruthy()
    ;(more as HTMLElement).click()
    await settle()
    expect(keys(recordType, 0)).toEqual(['name'])
    expect(keys(recordType, 1)).toEqual(['price'])
    expect(announced(el)).toBe('Price moved to More.')
  })

  it('places an unplaced field into a section', async () => {
    const recordType = reactive(item())
    const el = mount(recordType)
    expect(el.querySelector('.unplaced-tray')?.textContent).toContain('Notes')
    const triggers = [...el.querySelectorAll('.unplaced-row button')]
    expect(triggers).toHaveLength(1)
    ;(triggers[0] as HTMLButtonElement).click()
    await nextTick()
    await nextTick()
    const options = [...document.body.querySelectorAll('[role="menuitem"]')]
    const more = options.find((option) => option.textContent?.includes('More'))
    ;(more as HTMLElement).click()
    await settle()
    expect(keys(recordType, 1)).toEqual(['notes'])
    expect(el.querySelector('.unplaced-tray')).toBeNull()
  })

  it('arranges a form with no layout into one section', async () => {
    const recordType = reactive(item())
    delete recordType.layout
    const el = mount(recordType)
    expect(el.querySelector('.unplaced-tray')).toBeNull()
    ;(el.querySelector('.form-builder-main button') as HTMLButtonElement).click()
    await settle()
    // The delete above narrows the property away; the builder recreates it.
    expect((recordType.layout as FormLayout | undefined)?.sections).toHaveLength(1)
    expect(keys(recordType, 0)).toEqual(['name', 'price', 'notes'])
    expect(announced(el)).toContain('Arranged 3 fields')
  })

  it('reorders sections with up and down', async () => {
    const recordType = reactive(item())
    const el = mount(recordType)
    ;(el.querySelector('[data-section="1"] [data-action="up"]') as HTMLButtonElement).click()
    await settle()
    expect(recordType.layout?.sections.map((section) => section.title)).toEqual(['More', 'Basics'])
    expect(announced(el)).toBe('More moved up.')
  })
})
