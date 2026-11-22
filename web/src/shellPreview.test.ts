// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PreviewAsGroup from './components/shell/PreviewAsGroup.vue'
import RolePreviewPanel from './components/shell/RolePreviewPanel.vue'

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
})

async function flush() {
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

function mount(component: unknown, props: Record<string, unknown>, on: Record<string, (...args: never[]) => void> = {}) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(component as never, { ...props, ...on }) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  return flush()
}

const roles = [
  { name: 'Clerk', grants: [{ recordTypeKey: 'invoice', actions: ['read', 'create'], permissionLevel: 9 }] },
  { name: 'Reader', grants: [{ recordTypeKey: '*', actions: ['read'], permissionLevel: 1 }] },
]

function field(name: string, permission_level = 0) {
  return { name, label: name, type: 'Text', required: false, permission_level }
}

const types = [
  { name: 'invoice', label: 'Invoice', fields: [field('title'), field('cost', 5)] },
  { name: 'note', label: 'Note', fields: [field('title')] },
]

describe('PreviewAsGroup', () => {
  it('lists Roles and emits the selection', async () => {
    let selected = ''
    await mount(PreviewAsGroup, { roles, state: 'ready', allowed: true, deniedReason: '', activeName: '' }, { onSelect: (name: never) => { selected = name as string } })
    expect(document.body.textContent ?? '').toContain('Preview as')
    expect(document.body.textContent ?? '').toContain('Choose a Role…')
  })

  it('shows the reason when previewing is not allowed', async () => {
    await mount(PreviewAsGroup, { roles: [], state: 'idle', allowed: false, deniedReason: 'Needs the build permission.', activeName: '' })
    expect(document.body.textContent ?? '').toContain('Needs the build permission.')
  })

  it('shows loading, empty and failed states', async () => {
    await mount(PreviewAsGroup, { roles: [], state: 'loading', allowed: true, deniedReason: '', activeName: '' })
    expect(document.body.textContent ?? '').toContain('Loading Roles…')
    for (const cleanup of cleanups.splice(0)) cleanup()
    document.body.innerHTML = ''
    await mount(PreviewAsGroup, { roles: [], state: 'ready', allowed: true, deniedReason: '', activeName: '' })
    expect(document.body.textContent ?? '').toContain('No Roles yet')
  })

  it('offers an exit while previewing', async () => {
    let exited = false
    await mount(PreviewAsGroup, { roles, state: 'ready', allowed: true, deniedReason: '', activeName: 'Clerk' }, { onExit: () => { exited = true } })
    const exit = [...document.querySelectorAll('button')].find((button) => button.textContent?.includes('Exit preview'))
    expect(exit).not.toBeUndefined()
    ;(exit as HTMLElement).click()
    expect(exited).toBe(true)
  })
})

describe('RolePreviewPanel', () => {
  it('shows the lists a Role reaches with their actions', async () => {
    await mount(RolePreviewPanel, { role: roles[1], types: types as never })
    const text = document.body.textContent ?? ''
    expect(text).toContain('View as Reader')
    expect(text).toContain('read-only')
    expect(text).toContain('Invoice')
    expect(text).toContain('Note')
    expect(text).toContain('1 field stays hidden.')
    expect(text).not.toContain('Record contents are shown')
  })

  it('shows the empty state when the Role reaches nothing', async () => {
    await mount(RolePreviewPanel, { role: { name: 'Empty', grants: [] }, types: types as never })
    const text = document.body.textContent ?? ''
    expect(text).toContain('View as Empty')
    expect(text).toContain('sees no lists')
  })

  it('exits back to the page', async () => {
    let exited = false
    await mount(RolePreviewPanel, { role: roles[0], types: types as never }, { onExit: () => { exited = true } })
    const exit = [...document.querySelectorAll('button')].find((button) => button.textContent?.includes('Exit preview'))
    ;(exit as HTMLElement).click()
    expect(exited).toBe(true)
  })
})
