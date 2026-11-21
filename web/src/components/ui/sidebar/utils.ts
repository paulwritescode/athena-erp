import type { ComputedRef, Ref } from 'vue'
import { createContext } from 'reka-ui'

export const SIDEBAR_COOKIE_NAME = 'sidebar_state'
export const SIDEBAR_COOKIE_MAX_AGE = 60 * 60 * 24 * 7
// Athena adapts the stock widths and breakpoint to R2 section 7: a 240 px
// sidebar at >=1024, a 56 px icon rail from 640 to 1023, and the bottom
// navigation (not an offcanvas sidebar) below 640.
export const SIDEBAR_WIDTH = '15rem'
export const SIDEBAR_WIDTH_MOBILE = '18rem'
export const SIDEBAR_WIDTH_ICON = '3.5rem'
export const SIDEBAR_KEYBOARD_SHORTCUT = 'b'
export const SIDEBAR_MOBILE_BREAKPOINT = 640

export const [useSidebar, provideSidebarContext] = createContext<{
  state: ComputedRef<'expanded' | 'collapsed'>
  open: Ref<boolean>
  setOpen: (value: boolean) => void
  isMobile: Ref<boolean>
  openMobile: Ref<boolean>
  setOpenMobile: (value: boolean) => void
  toggleSidebar: () => void
}>('Sidebar')
