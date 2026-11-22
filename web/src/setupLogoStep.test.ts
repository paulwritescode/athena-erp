// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SetupLogoStep from './components/setup/SetupLogoStep.vue'
import { workspaceClient } from './api'

vi.mock('./api', () => ({
  workspaceClient: { getIdentity: vi.fn(), setIdentity: vi.fn() },
}))

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

async function mountStep(canWrite: boolean, onDone: (value: unknown) => void) {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ setup: () => () => h(SetupLogoStep, { canWrite, onDone }) })
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

function chooseFile(side: 'light' | 'dark', file: File) {
  const label = side === 'light' ? 'Choose a light logo file' : 'Choose a dark logo file'
  const input = document.querySelector(`input[aria-label="${label}"]`) as HTMLInputElement | null
  if (!input) throw new Error(`missing ${label}`)
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

const blank = { displayName: 'Acme', logoLightUrl: '', logoDarkUrl: '', favicon32Url: '', favicon180Url: '' }

describe('SetupLogoStep', () => {
  it('uploads a picked logo and continues', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...blank } } as never)
    vi.mocked(workspaceClient.setIdentity).mockResolvedValue({
      identity: { ...blank, logoLightUrl: '/brand/logo-aaa.png' },
    } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    chooseFile('light', new File(['png-bytes'], 'logo.png', { type: 'image/png' }))
    await new Promise((resolve) => setTimeout(resolve, 50))
    await nextTick()
    const preview = document.querySelector('img[alt="Light logo preview"]')?.getAttribute('src') ?? ''
    expect(preview.startsWith('data:')).toBe(true)
    button('Save logo & continue').click()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const set = vi.mocked(workspaceClient.setIdentity)
    expect(set).toHaveBeenCalledTimes(1)
    const req = set.mock.calls[0][0] as unknown as { displayName: string; logoLight?: { content: Uint8Array; filename: string } }
    expect(req.displayName).toBe('')
    expect(req.logoLight?.filename).toBe('logo.png')
    expect(new TextDecoder().decode(req.logoLight?.content)).toBe('png-bytes')
    expect(done).toEqual([{ hasLogo: true, skipped: false }])
  })

  it('rejects a bad extension inline', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...blank } } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    chooseFile('dark', new File(['x'], 'logo.jpg', { type: 'image/jpeg' }))
    await nextTick()
    expect(document.body.textContent).toContain('is not a logo')
    expect(vi.mocked(workspaceClient.setIdentity)).not.toHaveBeenCalled()
    expect(done).toEqual([])
  })

  it('skips without a logo', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...blank } } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    button('Skip for now').click()
    await nextTick()
    expect(vi.mocked(workspaceClient.setIdentity)).not.toHaveBeenCalled()
    expect(done).toEqual([{ hasLogo: false, skipped: true }])
  })

  it('continues when a logo already exists', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({
      identity: { ...blank, logoLightUrl: '/brand/logo-aaa.png' },
    } as never)
    const done: unknown[] = []
    await mountStep(true, (value) => done.push(value))
    expect(document.querySelector('img[alt="Light logo preview"]')?.getAttribute('src')).toBe('/brand/logo-aaa.png')
    button('Continue').click()
    await nextTick()
    expect(done).toEqual([{ hasLogo: true, skipped: false }])
  })
})
