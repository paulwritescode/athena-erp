// Workspace identity (B5): the display name and logos the header, sign-in,
// tab title and favicon wear. The state loads once at boot over the public
// GetIdentity call and refreshes after a Settings save. A missing logo falls
// back to the Athena mark; the favicon URLs always resolve because the
// server falls back to the mark itself.
import { ref } from 'vue'
import { workspaceClient } from './api'
import type { Identity } from './gen/athena/v1/workspace_pb'

export type IdentityState = {
  displayName: string
  logoLightUrl: string
  logoDarkUrl: string
  favicon32Url: string
  favicon180Url: string
}

export const blankIdentity: IdentityState = { displayName: '', logoLightUrl: '', logoDarkUrl: '', favicon32Url: '', favicon180Url: '' }

/** The current identity; components read it directly and re-render on change. */
export const identity = ref<IdentityState>({ ...blankIdentity })

function fromMessage(message: Identity | undefined): IdentityState {
  return {
    displayName: message?.displayName ?? '',
    logoLightUrl: message?.logoLightUrl ?? '',
    logoDarkUrl: message?.logoDarkUrl ?? '',
    favicon32Url: message?.favicon32Url ?? '',
    favicon180Url: message?.favicon180Url ?? '',
  }
}

/** True once the Workspace sets either logo. */
export function hasLogo(state: IdentityState): boolean {
  return state.logoLightUrl !== '' || state.logoDarkUrl !== ''
}

/** The tab title: the Workspace name, or Athena when none is set. */
export function identityTitle(state: IdentityState): string {
  return state.displayName ? `${state.displayName} - Athena` : 'Athena'
}

/** The logo URL for a theme, or '' for the Athena mark. A lone light logo serves both themes. */
export function logoUrl(state: IdentityState, theme: 'light' | 'dark'): string {
  if (theme === 'dark') return state.logoDarkUrl || state.logoLightUrl
  return state.logoLightUrl
}

function setLink(rel: string, href: string, extra: Record<string, string> = {}): void {
  if (!href) return
  let link = document.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`)
  if (!link) {
    link = document.createElement('link')
    link.rel = rel
    document.head.appendChild(link)
  }
  link.href = href
  for (const [key, value] of Object.entries(extra)) link.setAttribute(key, value)
}

/** Applies the tab title and favicons for an identity. */
export function applyIdentity(state: IdentityState): void {
  document.title = identityTitle(state)
  setLink('icon', state.favicon32Url, { type: 'image/png', sizes: '32x32' })
  setLink('apple-touch-icon', state.favicon180Url)
}

/** Loads the identity and applies it. Failures keep whatever stands. */
export async function refreshIdentity(): Promise<void> {
  try {
    const res = await workspaceClient.getIdentity({})
    const next = fromMessage(res.identity)
    identity.value = next
    applyIdentity(next)
  } catch {
    // The mark, the default title and the static icons stand in.
  }
}
