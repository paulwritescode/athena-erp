// @vitest-environment jsdom
// The Reports page lists saved lists, runs the open one, exports its PDF,
// and previews the message audience before queueing (F4 slice 3). The list
// client is mocked; these tests pin the page wiring, not the server.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { listClient } from '@/api'
import ReportsPage from './ReportsPage.vue'
import type { RecordType } from '@/types'

vi.mock('@/api', () => ({
  listClient: {
    listLists: vi.fn(),
    runList: vi.fn(),
    saveList: vi.fn(),
    deleteList: vi.fn(),
    exportListPdf: vi.fn(),
    queueMessages: vi.fn(),
  },
}))
vi.mock('@/lookup', () => ({ resolveRecords: vi.fn(async () => new Map()), labelWithState: (r: { label?: string }) => r.label ?? '' }))
vi.mock('vue-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

// The hairline figures observe intersection; jsdom has no
// IntersectionObserver, so the figure mount throws. A no-op observer keeps
// the art mount quiet (same stub as pageSwitchBuild.test.ts).
class NoopIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(window, 'IntersectionObserver', { configurable: true, value: NoopIntersectionObserver })
Object.defineProperty(globalThis, 'IntersectionObserver', { configurable: true, value: NoopIntersectionObserver })
Object.defineProperty(window, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: false,
    media: '',
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
    onchange: null,
  }),
})

const cleanups: Array<() => void> = []

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const guests: RecordType = {
  name: 'guests',
  label: 'Guests',
  description: '',
  kind: 'document',
  fields: [
    { name: 'name', label: 'Name', type: 'ShortText', required: true },
    { name: 'rsvp', label: 'RSVP', type: 'Enum', required: false, values: [{ value: 'yes', label: 'Yes' }, { value: 'no', label: 'No' }] },
  ],
  revision: 1,
}

const savedList = {
  id: 'l1',
  name: 'Veg yeses',
  recordTypeKey: 'guests',
  conditionTree: '{"root":{"all":true,"conditions":[]}}',
  conditionExpr: 'true',
  columns: [],
  owner: 'root',
  revision: 1n,
}

function mountPage(props: { canManage: boolean; canSend: boolean } = { canManage: true, canSend: true }) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp({ render: () => h(ReportsPage as never, { ...props, recordTypes: [guests] }) })
  app.mount(host)
  cleanups.push(() => app.unmount())
  return host
}

async function settled(times = 6) {
  for (let i = 0; i < times; i += 1) {
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
}

// The dialogs teleport to the body, so buttons are searched from the
// document, not the mount host.
function buttonByText(host: HTMLElement, text: string): HTMLButtonElement {
  const found = [...document.querySelectorAll('button')].find((b) => (b.textContent ?? '').includes(text))
  if (!found) throw new Error(`button ${text} missing: ${document.body.innerHTML.slice(0, 400)}`)
  return found as HTMLButtonElement
}

describe('ReportsPage', () => {
  it('lists the saved lists of the selected type', async () => {
    vi.mocked(listClient.listLists).mockResolvedValue({ lists: [savedList], nextPageToken: '' } as never)
    const host = mountPage()
    await settled()
    expect(host.textContent).toContain('Veg yeses')
    expect(listClient.listLists).toHaveBeenCalledWith(expect.objectContaining({ recordTypeKey: 'guests' }))
  })

  it('opens a list and shows its rows', async () => {
    vi.mocked(listClient.listLists).mockResolvedValue({ lists: [savedList], nextPageToken: '' } as never)
    vi.mocked(listClient.runList).mockResolvedValue({
      list: savedList,
      records: [{ id: 'r1', values: { name: { kind: { case: 'stringValue', value: 'Ayo' } } } }],
      nextPageToken: '',
    } as never)
    const host = mountPage()
    await settled()
    buttonByText(host, 'Veg yeses').click()
    await settled()
    expect(listClient.runList).toHaveBeenCalledWith(expect.objectContaining({ id: 'l1' }))
    expect(host.textContent).toContain('Ayo')
  })

  it('exports the open list to PDF', async () => {
    vi.mocked(listClient.listLists).mockResolvedValue({ lists: [savedList], nextPageToken: '' } as never)
    vi.mocked(listClient.runList).mockResolvedValue({ list: savedList, records: [], nextPageToken: '' } as never)
    vi.mocked(listClient.exportListPdf).mockResolvedValue({ pdf: new Uint8Array([1, 2, 3]), filename: 'veg-yeses.pdf' } as never)
    const created: string[] = []
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => {
        created.push('blob:pdf')
        return 'blob:pdf'
      }),
      revokeObjectURL: vi.fn(),
    })
    const host = mountPage()
    await settled()
    buttonByText(host, 'Veg yeses').click()
    await settled()
    buttonByText(host, 'Export PDF').click()
    await settled()
    expect(listClient.exportListPdf).toHaveBeenCalledWith(expect.objectContaining({ id: 'l1' }))
    expect(created).toHaveLength(1)
  })

  it('previews the send before queueing, dry-run first', async () => {
    vi.mocked(listClient.listLists).mockResolvedValue({ lists: [savedList], nextPageToken: '' } as never)
    vi.mocked(listClient.runList).mockResolvedValue({ list: savedList, records: [], nextPageToken: '' } as never)
    vi.mocked(listClient.queueMessages)
      .mockResolvedValueOnce({ queued: 3n, excluded: 1n, total: 4n } as never)
      .mockResolvedValueOnce({ queued: 3n, excluded: 1n, total: 4n } as never)
    const host = mountPage()
    await settled()
    buttonByText(host, 'Veg yeses').click()
    await settled()
    buttonByText(host, 'Email them').click()
    await settled()
    const queue = buttonByText(host, 'Queue')
    expect(queue.disabled).toBe(true)
    buttonByText(host, 'Preview counts').click()
    await settled()
    expect(listClient.queueMessages).toHaveBeenCalledWith(expect.objectContaining({ dryRun: true }))
    expect(document.body.textContent).toContain('3')
    expect(document.body.textContent).toContain('4 matching rows will be queued')
    buttonByText(host, 'Queue 3').click()
    await settled()
    expect(listClient.queueMessages).toHaveBeenLastCalledWith(expect.not.objectContaining({ dryRun: true }))
  })

  it('hides the manage and send actions without the permissions', async () => {
    vi.mocked(listClient.listLists).mockResolvedValue({ lists: [savedList], nextPageToken: '' } as never)
    vi.mocked(listClient.runList).mockResolvedValue({ list: savedList, records: [], nextPageToken: '' } as never)
    const host = mountPage({ canManage: false, canSend: false })
    await settled()
    expect(host.textContent).not.toContain('New list')
    buttonByText(host, 'Veg yeses').click()
    await settled()
    expect(host.textContent).not.toContain('Email them')
    expect(host.textContent).toContain('Export PDF')
  })
})
