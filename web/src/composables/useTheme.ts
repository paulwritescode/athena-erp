// Light/dark theme. The choice persists in localStorage; without a stored
// choice the desk follows the operating system. The class on <html> drives
// the token swap in theme.css.
import { ref } from 'vue'

export type ThemeState = ReturnType<typeof useTheme>

export function useTheme() {
  const isDark = ref(false)

  function init() {
    const stored = localStorage.getItem('athena-theme')
    isDark.value = stored ? stored === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
    document.documentElement.classList.toggle('dark', isDark.value)
  }

  function toggle() {
    isDark.value = !isDark.value
    document.documentElement.classList.toggle('dark', isDark.value)
    localStorage.setItem('athena-theme', isDark.value ? 'dark' : 'light')
  }

  return { isDark, init, toggle }
}
