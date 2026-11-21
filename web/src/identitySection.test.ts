// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import IdentitySection from './components/settings/IdentitySection.vue'
import { workspaceClient } from './api'
import { identity } from './identity'

vi.mock('./api', () => ({
  workspaceClient: { getIdentity: vi.fn(), setIdentity: vi.fn() },
}))

const stored = {
  displayName: 'Acme',
  logoLightUrl: '/brand/logo-aaa.png',
  logoDarkUrl: '',
  favicon32Url: '/brand/favicon-aaa-32.png',
  favicon180Url: '/brand/favicon-aaa-180.png',
}

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function mountSection(canEdit: boolean) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(IdentitySection, { canEdit }) })
  app.mount(el)
  cleanups.push(() => app.unmount())
  await nextTick()
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
}

function button(label: string): HTMLButtonElement {
  const found = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes(label))
  if (!found) throw new Error(`missing button ${label}`)
  return found as HTMLButtonElement
}

async function setName(value: string) {
  const input = document.querySelector('#identity-name') as HTMLInputElement | null
  if (!input) throw new Error('missing identity-name')
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

function chooseFile(side: 'light' | 'dark', file: File) {
  const label = side === 'light' ? 'Choose a light logo file' : 'Choose a dark logo file'
  const input = document.querySelector(`input[aria-label="${label}"]`) as HTMLInputElement | null
  if (!input) throw new Error(`missing ${label}`)
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

describe('IdentitySection', () => {
  it('loads the name and current logos', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...stored } } as never)
    await mountSection(true)
    expect((document.querySelector('#identity-name') as HTMLInputElement).value).toBe('Acme')
    expect(document.querySelector('img[alt="Light logo preview"]')?.getAttribute('src')).toBe('/brand/logo-aaa.png')
    expect(document.body.textContent).toContain('No logo')
  })

  it('is read-only without the permission', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...stored } } as never)
    await mountSection(false)
    expect(document.body.textContent).toContain('You need the workspace permission to change this.')
    expect([...document.querySelectorAll('button')].some((b) => b.textContent?.includes('Save identity'))).toBe(false)
  })

  it('rejects a bad extension and an oversized file', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...stored } } as never)
    await mountSection(true)
    chooseFile('dark', new File(['x'], 'logo.jpg', { type: 'image/jpeg' }))
    await nextTick()
    expect(document.body.textContent).toContain('is not a logo')
    chooseFile('dark', new File([new Uint8Array(512 * 1024 + 1)], 'big.png', { type: 'image/png' }))
    await nextTick()
    expect(document.body.textContent).toContain('larger than 512 KB')
  })

  it('saves the name and upload bytes, then refreshes', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...stored } } as never)
    vi.mocked(workspaceClient.setIdentity).mockImplementation(async (req) => {
      return { identity: { ...stored, displayName: req.displayName || stored.displayName } } as never
    })
    await mountSection(true)
    await setName('Acme Ltd')
    chooseFile('dark', new File(['svg-bytes'], 'logo.svg', { type: 'image/svg+xml' }))
    await new Promise((resolve) => setTimeout(resolve, 50))
    await nextTick()
    const preview = document.querySelector('img[alt="Dark logo preview"]')?.getAttribute('src') ?? ''
    expect(preview.startsWith('data:')).toBe(true)
    button('Save identity').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const set = vi.mocked(workspaceClient.setIdentity)
    expect(set).toHaveBeenCalledTimes(1)
    const req = set.mock.calls[0][0] as unknown as { displayName: string; logoDark?: { content: Uint8Array; filename: string }; clearLogoDark: boolean }
    expect(req.displayName).toBe('Acme Ltd')
    expect(req.logoDark?.filename).toBe('logo.svg')
    expect(new TextDecoder().decode(req.logoDark?.content)).toBe('svg-bytes')
    expect(req.clearLogoDark).toBe(false)
    expect(vi.mocked(workspaceClient.getIdentity).mock.calls.length).toBeGreaterThan(1)
    expect(identity.value.displayName).toBe('Acme')
  })

  it('sends the clear flag when a logo is removed', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...stored } } as never)
    vi.mocked(workspaceClient.setIdentity).mockResolvedValue({ identity: { ...stored, logoLightUrl: '' } } as never)
    await mountSection(true)
    button('Remove').click()
    await nextTick()
    button('Save identity').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const req = vi.mocked(workspaceClient.setIdentity).mock.calls[0][0] as unknown as { clearLogoLight: boolean }
    expect(req.clearLogoLight).toBe(true)
  })
})
