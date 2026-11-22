// @vitest-environment jsdom
// Startup mode: a fresh load of ?mode=build lands in Build (B8 regression
// test). Before B2 the canBuildDesk watch had no session guard, so the
// mid-startup transition to "database connected, Grants unknown" flipped the
// mode to View before the session arrived. The guard stays quiet until the
// session is known; these tests pin that behavior.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useDeskRoute } from './composables/useDeskRoute'
import { useSession } from './composables/useSession'
import { authClient, systemClient, workspaceClient } from '@/api'

vi.mock('@/api', () => ({
  authClient: { getSession: vi.fn(), logout: vi.fn() },
  systemClient: { getSystemInfo: vi.fn() },
  workspaceClient: { getAppearance: vi.fn() },
  onSessionEnded: vi.fn(),
  setCsrfToken: vi.fn(),
}))
vi.mock('@/identity', () => ({ refreshIdentity: vi.fn() }))
vi.mock('@/appearance', () => ({ applyAppearance: vi.fn(), saveCachedAppearance: vi.fn() }))
vi.mock('@/idleWarning', () => ({
  clearIdleWarning: vi.fn(), noteAlive: vi.fn(), rescheduleIdleWarning: vi.fn(),
  setIdleSignOut: vi.fn(), startIdleTracking: vi.fn(),
}))
vi.mock('@/sessionChannel', () => ({ onSessionMessage: vi.fn(), postSessionMessage: vi.fn() }))
vi.mock('@/telemetry', () => ({ startBrowserTelemetry: vi.fn() }))
vi.mock('vue-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

function fakeWindow(pathname = '/', search = '') {
  const location = { pathname, search }
  const apply = (url: string) => {
    const parsed = new URL(url, 'http://desk.test')
    location.pathname = parsed.pathname
    location.search = parsed.search
  }
  vi.stubGlobal('window', {
    location,
    history: {
      pushState: (_d: unknown, _t: string, url: string) => apply(url),
      replaceState: (_d: unknown, _t: string, url: string) => apply(url),
    },
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    matchMedia: () => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {} }),
  })
  return location
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function settled(times = 5) {
  for (let i = 0; i < times; i += 1) {
    await nextTick()
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
}

function builderSession() {
  return { authenticated: true, username: 'amy', displayName: 'Amy', owner: false, systemPermissions: ['build'], csrfToken: 'x', idleExpiresAt: '' }
}

function viewerSession() {
  return { authenticated: true, username: 'bob', displayName: 'Bob', owner: false, systemPermissions: [], csrfToken: 'x', idleExpiresAt: '' }
}

function stubServer(session: unknown) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ json: () => Promise.resolve({ database: 'Connected' }) }))
  vi.mocked(systemClient.getSystemInfo).mockResolvedValue({ workspaceName: 'W', telemetryEnabled: false } as never)
  vi.mocked(workspaceClient.getAppearance).mockResolvedValue({} as never)
  vi.mocked(authClient.getSession).mockResolvedValue({ hasUsers: true, session } as never)
}

function startup(search: string) {
  fakeWindow('/', search)
  const route = useDeskRoute()
  const library = { load: vi.fn().mockResolvedValue(undefined), clear: vi.fn() }
  const session = useSession(route, library as never)
  return { route, session }
}

describe('startup mode', () => {
  it('lands a restored builder session in Build on ?mode=build', async () => {
    stubServer(builderSession())
    const { route, session } = startup('?mode=build')
    await session.startDesk()
    await settled()
    expect(route.mode.value).toBe('build')
    expect(route.page.value).toBe('overview')
  })

  it('keeps ?mode=build while the session is still arriving', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ json: () => Promise.resolve({ database: 'Connected' }) }))
    vi.mocked(systemClient.getSystemInfo).mockResolvedValue({ workspaceName: 'W', telemetryEnabled: false } as never)
    vi.mocked(workspaceClient.getAppearance).mockResolvedValue({} as never)
    let resolveSession!: (value: never) => void
    vi.mocked(authClient.getSession).mockReturnValue(new Promise((resolve) => { resolveSession = resolve as (value: never) => void }))
    const { route, session } = startup('?mode=build')
    const started = session.startDesk()
    await settled()
    // The database is connected but the Grants are unknown: the mode must
    // not flip before the session arrives.
    expect(route.mode.value).toBe('build')
    resolveSession({ hasUsers: true, session: builderSession() } as never)
    await started
    await settled()
    expect(route.mode.value).toBe('build')
  })

  it('lands a fresh sign-in in Build on ?mode=build', async () => {
    stubServer(undefined)
    const { route, session } = startup('?mode=build')
    await session.startDesk()
    await settled()
    expect(route.mode.value).toBe('build')
    await session.signedIn(builderSession() as never)
    await settled()
    expect(route.mode.value).toBe('build')
  })

  it('keeps an explicit ?mode=build for a viewer (the page switch denies it)', async () => {
    stubServer(viewerSession())
    const { route, session } = startup('?mode=build')
    await session.startDesk()
    await settled()
    expect(route.mode.value).toBe('build')
  })

  it('starts a viewer in View on a bare address', async () => {
    stubServer(viewerSession())
    const { route, session } = startup('')
    await session.startDesk()
    await settled()
    expect(route.mode.value).toBe('view')
  })

  it('starts a builder in Build on a bare address', async () => {
    stubServer(builderSession())
    const { route, session } = startup('')
    await session.startDesk()
    await settled()
    expect(route.mode.value).toBe('build')
  })
})
