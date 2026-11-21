// Light/dark theme. The choice is per user: light, dark or system, persisted
// in localStorage (an old light/dark value migrates as-is; nothing stored
// means system). The class on <html> drives the token swap in theme.css,
// and system follows the operating system live.
import { computed, ref } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'system'
export type ThemeState = ReturnType<typeof useTheme>

const KEY = 'athena-theme'

export function useTheme() {
  const mode = ref<ThemeMode>('system')
  const isDark = ref(false)
  let media: MediaQueryList | null = null

  function resolve() {
    isDark.value = mode.value === 'dark' || (mode.value === 'system' && (media?.matches ?? false))
    document.documentElement.classList.toggle('dark', isDark.value)
  }

  function onSystemChange() {
    if (mode.value === 'system') resolve()
  }

  function init() {
    const stored = localStorage.getItem(KEY)
    mode.value = stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
    media = window.matchMedia('(prefers-color-scheme: dark)')
    media.addEventListener('change', onSystemChange)
    resolve()
  }

  function setMode(next: ThemeMode) {
    mode.value = next
    localStorage.setItem(KEY, next)
    resolve()
  }

  // The header button cycles; the user menu names each choice.
  function toggle() {
    setMode(mode.value === 'light' ? 'dark' : mode.value === 'dark' ? 'system' : 'light')
  }

  const modeLabel = computed(() => (mode.value === 'light' ? 'Light' : mode.value === 'dark' ? 'Dark' : 'System'))

  return { isDark, mode, modeLabel, init, setMode, toggle }
}
