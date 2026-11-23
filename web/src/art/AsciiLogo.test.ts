// @vitest-environment jsdom
// AsciiLogo draws the frames top to bottom on a timer, plays once and
// rests; reduced motion renders the full mark at once. Unknown keys and
// figure keys render nothing.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import AsciiLogo from './AsciiLogo.vue'
import { LOGO_DARK_FRAMES, LOGO_LIGHT_FRAMES, LOGO_STEP_MS } from './logo-frames'

type Mounted = { el: HTMLElement; unmount: () => void }

const mounted: Mounted[] = []
const realMatchMedia = window.matchMedia

afterEach(() => {
  for (const m of mounted.splice(0)) m.unmount()
  document.body.innerHTML = ''
  vi.useRealTimers()
  if (realMatchMedia) window.matchMedia = realMatchMedia
  else delete (window as { matchMedia?: unknown }).matchMedia
})

function mockMatchMedia(reduced: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: reduced && query.includes('prefers-reduced-motion'),
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia
}

function mountLogo(artKey: string): Mounted {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp({ render: () => h(AsciiLogo, { artKey }) })
  app.mount(el)
  const tracked: Mounted = {
    el,
    unmount: () => {
      app.unmount()
      el.remove()
    },
  }
  mounted.push(tracked)
  return tracked
}

/** The light frame's rows, with blank rows as a single space. */
function lightRows(el: HTMLElement): string[] {
  const pre = el.querySelector('pre.dark\\:hidden')
  if (!pre) return []
  return [...pre.querySelectorAll('.ascii-logo-line')].map((span) => span.textContent ?? '')
}

describe('AsciiLogo', () => {
  it('renders nothing for unlisted keys and figure keys', () => {
    mockMatchMedia(false)
    for (const key of ['no-such-placement', 'signin']) {
      const { el } = mountLogo(key)
      expect(el.querySelector('.ascii-logo'), key).toBeNull()
    }
  })

  it('draws the light mark top to bottom, then rests', async () => {
    mockMatchMedia(false)
    vi.useFakeTimers()
    const { el } = mountLogo('signin-logo')
    await nextTick()
    const frames = LOGO_LIGHT_FRAMES
    expect(lightRows(el)).toEqual(frames[0]!.map((line) => (line === '' ? ' ' : line)))
    for (let k = 1; k < frames.length; k++) {
      vi.advanceTimersByTime(LOGO_STEP_MS)
      await nextTick()
      expect(lightRows(el)).toEqual(frames[k]!.map((line) => (line === '' ? ' ' : line)))
    }
    const rested = lightRows(el)
    vi.advanceTimersByTime(LOGO_STEP_MS * 10)
    await nextTick()
    expect(lightRows(el)).toEqual(rested)
    expect(rested.join('')).toContain('#')
  })

  it('renders the dark variant alongside the light one', async () => {
    mockMatchMedia(false)
    vi.useFakeTimers()
    const { el } = mountLogo('offline-logo')
    await nextTick()
    vi.advanceTimersByTime(LOGO_STEP_MS * LOGO_DARK_FRAMES.length)
    await nextTick()
    const dark = el.querySelector('pre.dark\\:block')
    expect(dark).not.toBeNull()
    const rows = [...dark!.querySelectorAll('.ascii-logo-line')].map((span) => span.textContent ?? '')
    const final = LOGO_DARK_FRAMES[LOGO_DARK_FRAMES.length - 1]!
    expect(rows).toEqual(final.map((line) => (line === '' ? ' ' : line)))
  })

  it('renders the full mark at once under reduced motion', async () => {
    mockMatchMedia(true)
    vi.useFakeTimers()
    const { el } = mountLogo('not-found-logo')
    await nextTick()
    const final = LOGO_LIGHT_FRAMES[LOGO_LIGHT_FRAMES.length - 1]!
    expect(lightRows(el)).toEqual(final.map((line) => (line === '' ? ' ' : line)))
  })

  it('stays out of the access tree', async () => {
    mockMatchMedia(true)
    const { el } = mountLogo('first-run-logo')
    await nextTick()
    expect(el.querySelector('.ascii-logo')?.getAttribute('aria-hidden')).toBe('true')
  })
})
