// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useTheme } from './composables/useTheme'

afterEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  vi.unstubAllGlobals()
})

function studioSystem(dark: boolean) {
  const query = {
    matches: dark,
    media: '',
    listeners: [] as (() => void)[],
    addEventListener(_event: string, cb: () => void) { query.listeners.push(cb) },
    removeEventListener() {},
    dispatchEvent() { return false },
  }
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: vi.fn(() => query) })
  return query
}

describe('useTheme', () => {
  it('starts on the stored choice, or the system when nothing is stored', () => {
    studioSystem(true)
    localStorage.setItem('athena-theme', 'light')
    const explicit = useTheme()
    explicit.init()
    expect(explicit.mode.value).toBe('light')
    expect(explicit.isDark.value).toBe(false)

    localStorage.clear()
    document.documentElement.classList.remove('dark')
    const system = useTheme()
    system.init()
    expect(system.mode.value).toBe('system')
    expect(system.isDark.value).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('follows the operating system live in system mode only', () => {
    const query = studioSystem(false)
    const theme = useTheme()
    theme.init()
    expect(theme.isDark.value).toBe(false)
    query.matches = true
    for (const cb of query.listeners) cb()
    expect(theme.isDark.value).toBe(true)
    theme.setMode('light')
    expect(localStorage.getItem('athena-theme')).toBe('light')
    query.matches = false
    for (const cb of query.listeners) cb()
    expect(theme.isDark.value).toBe(false)
  })

  it('cycles light, dark and system from the header button', () => {
    studioSystem(false)
    const theme = useTheme()
    theme.init()
    theme.setMode('light')
    theme.toggle()
    expect(theme.mode.value).toBe('dark')
    theme.toggle()
    expect(theme.mode.value).toBe('system')
    theme.toggle()
    expect(theme.mode.value).toBe('light')
    expect(theme.modeLabel.value).toBe('Light')
  })
})
