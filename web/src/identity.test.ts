// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { applyIdentity, blankIdentity, hasLogo, identity, identityTitle, logoUrl, refreshIdentity, type IdentityState } from './identity'
import { workspaceClient } from './api'

vi.mock('./api', () => ({
  workspaceClient: { getIdentity: vi.fn(), setIdentity: vi.fn() },
}))

const named: IdentityState = {
  displayName: 'Acme',
  logoLightUrl: '/brand/logo-aaa.png',
  logoDarkUrl: '/brand/logo-bbb.svg',
  favicon32Url: '/brand/favicon-aaa-32.png',
  favicon180Url: '/brand/favicon-aaa-180.png',
}

afterEach(() => {
  document.head.innerHTML = ''
  document.title = ''
  identity.value = { ...blankIdentity }
  vi.restoreAllMocks()
})

describe('identity helpers', () => {
  it('titles the tab with the Workspace name', () => {
    expect(identityTitle(named)).toBe('Acme - Athena')
    expect(identityTitle(blankIdentity)).toBe('Athena')
  })

  it('picks the theme logo with mark fallback', () => {
    expect(logoUrl(named, 'light')).toBe('/brand/logo-aaa.png')
    expect(logoUrl(named, 'dark')).toBe('/brand/logo-bbb.svg')
    expect(logoUrl({ ...named, logoDarkUrl: '' }, 'dark')).toBe('/brand/logo-aaa.png')
    expect(logoUrl(blankIdentity, 'light')).toBe('')
    expect(logoUrl(blankIdentity, 'dark')).toBe('')
  })

  it('reports whether a logo is set', () => {
    expect(hasLogo(named)).toBe(true)
    expect(hasLogo({ ...named, logoLightUrl: '', logoDarkUrl: '' })).toBe(false)
  })

  it('applies the title and favicon links', () => {
    applyIdentity(named)
    expect(document.title).toBe('Acme - Athena')
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/brand/favicon-aaa-32.png')
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('sizes')).toBe('32x32')
    expect(document.querySelector('link[rel="apple-touch-icon"]')?.getAttribute('href')).toBe('/brand/favicon-aaa-180.png')
  })

  it('reuses existing link tags instead of duplicating them', () => {
    document.head.innerHTML = '<link rel="icon" href="/old.png"><link rel="apple-touch-icon" href="/old180.png">'
    applyIdentity(named)
    expect(document.querySelectorAll('link[rel="icon"]')).toHaveLength(1)
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/brand/favicon-aaa-32.png')
  })

  it('loads the identity and applies it', async () => {
    vi.mocked(workspaceClient.getIdentity).mockResolvedValue({ identity: { ...named } } as never)
    await refreshIdentity()
    expect(identity.value).toEqual(named)
    expect(document.title).toBe('Acme - Athena')
  })

  it('keeps the standing state when loading fails', async () => {
    identity.value = { ...named }
    document.title = 'Acme - Athena'
    vi.mocked(workspaceClient.getIdentity).mockRejectedValue(new Error('down'))
    await refreshIdentity()
    expect(identity.value).toEqual(named)
    expect(document.title).toBe('Acme - Athena')
  })
})
