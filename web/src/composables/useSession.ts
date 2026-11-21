// Sign-in state, Grants-derived permissions, and desk startup. The desk shows
// the sign-in page until the server reports a session; every call is checked
// again on the server. Behaviour matches the pre-B2 App.vue exactly.
import { computed, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { classify, describeFailure, sessionEndNote, type Failure } from '@/apiErrors'
import { authClient, onSessionEnded, setCsrfToken, systemClient, workspaceClient } from '@/api'
import { applyAppearance, saveCachedAppearance } from '@/appearance'
import { clearIdleWarning, noteAlive, rescheduleIdleWarning, setIdleSignOut, startIdleTracking } from '@/idleWarning'
import { onSessionMessage, postSessionMessage } from '@/sessionChannel'
import type { SessionInfo } from '@/gen/athena/v1/auth_pb'
import type { SessionUser } from '@/types'
import { visibleBuildKeys } from '@/pages/registry'
import type { DeskRoute } from './useDeskRoute'
import type { RecordTypeLibrary } from './useRecordTypes'
import { startBrowserTelemetry } from '@/telemetry'

export type SessionState = ReturnType<typeof useSession>

export function useSession(route: DeskRoute, library: RecordTypeLibrary) {
  const apiState = ref<'connecting' | 'connected' | 'error'>('connecting')
  const databaseState = ref('Checking')
  const workspaceName = ref('')

  // Sign-in state.
  const authState = ref<'checking' | 'signed-out' | 'signed-in'>('checking')
  const hasUsers = ref(true)
  // What the sign-in card shows when the desk cannot start normally.
  const pageState = ref<Failure | null>(null)
  const signInNote = ref('')
  const rechecking = ref(false)
  const user = ref<SessionUser | null>(null)
  const changingPassword = ref(false)

  // Build needs the build permission. Everyone else works in View only. Without
  // a database there is no session, and the desk keeps both modes.
  const canBuild = computed(() => (user.value ? user.value.systemPermissions.includes('build') : databaseState.value !== 'Connected'))
  // Files has its own permissions, so the nav item follows files:read and the
  // page edits only with files:write. The server checks both on every call.
  const canFiles = computed(() => (user.value ? user.value.systemPermissions.includes('files:read') : false))
  const canWriteFiles = computed(() => (user.value ? user.value.systemPermissions.includes('files:write') : false))
  // Rules follow rules:read and the page edits only with rules:write; without a
  // database there is no session and the page shows why it is unavailable.
  const canRules = computed(() => (user.value ? user.value.systemPermissions.includes('rules:read') : databaseState.value !== 'Connected'))
  const canWriteRules = computed(() => (user.value ? user.value.systemPermissions.includes('rules:write') : false))
  // Access follows access:users or access:roles; the server checks the finer
  // access:* permissions on every call.
  const canAccess = computed(() => (user.value ? user.value.systemPermissions.includes('access:users') || user.value.systemPermissions.includes('access:roles') : false))
  // Ledger follows ledger:read and edits only with ledger:write.
  const canLedger = computed(() => (user.value ? user.value.systemPermissions.includes('ledger:read') : false))
  const canWriteLedger = computed(() => (user.value ? user.value.systemPermissions.includes('ledger:write') : false))
  // Build opens for the build permission, files:read, rules:read, access or
  // ledger:read; a user with only the last four sees just those pages. Newer
  // Build pages (Reports, Jobs, Settings, Setup, Gallery) need Build mode but
  // do not open it on their own.
  const canBuildDesk = computed(() => canBuild.value || canFiles.value || canRules.value || canAccess.value || canLedger.value)

  // A lost session or lost Grants drop a signed-in user to View. The guard
  // stays quiet until the session is known: mid-startup transitions must not
  // rewrite a deep link before its Grants arrive.
  watch(canBuildDesk, (value) => { if (!value && authState.value === 'signed-in') route.setMode('view') })

  // A user without build who landed on a bare address goes to the first
  // Build page they have. An explicit address keeps its page and mode: the
  // page switch answers with PermissionDenied when the Grants do not reach
  // it. Signed out, nothing is visible and the page stays put.
  function landOnBuildPage() {
    if (canBuild.value || route.landedExplicit) return
    const visible = visibleBuildKeys(canBuild.value, user.value?.systemPermissions ?? [], user.value !== null)
    const first = visible[0]
    if (first) route.page.value = first
  }

  function applySession(info: SessionInfo | undefined) {
    if (info?.authenticated) {
      setCsrfToken(info.csrfToken)
      user.value = { username: info.username, displayName: info.displayName, owner: info.owner, systemPermissions: info.systemPermissions }
      authState.value = 'signed-in'
      rescheduleIdleWarning(info.idleExpiresAt)
    } else {
      setCsrfToken('')
      user.value = null
      library.clear()
      authState.value = 'signed-out'
      clearIdleWarning()
    }
  }

  async function signedIn(info: SessionInfo) {
    signInNote.value = ''
    applySession(info)
    toast.success(`Signed in as ${info.displayName || info.username}.`)
    forceViewWithoutDesk()
    landOnBuildPage()
    await library.load()
  }

  // Without Build access and without an explicit address, a known session
  // starts in View. An explicit deep link keeps its mode: the page switch
  // answers with PermissionDenied when the Grants do not reach it.
  function forceViewWithoutDesk() {
    if (authState.value === 'signed-in' && !canBuildDesk.value && !route.landedExplicit) route.setMode('view')
  }

  async function signOut() {
    try {
      await authClient.logout({})
    } catch (err) {
      toast.error(err instanceof ConnectError ? `Could not sign out: ${err.rawMessage}` : 'Could not sign out.')
      return
    }
    postSessionMessage('signed-out')
    applySession(undefined)
    toast.success('Signed out.')
  }

  onSessionEnded((reason) => {
    if (authState.value !== 'signed-in') return
    applySession(undefined)
    signInNote.value = sessionEndNote(reason)
  })

  // Same-browser tabs stay in step: a sign-out, Workspace change or access
  // change in one reaches the others within 2 seconds.
  onSessionMessage((message) => {
    if (message.kind === 'signed-out') {
      if (authState.value !== 'signed-in') return
      applySession(undefined)
      signInNote.value = 'Signed out in another tab.'
    } else if (message.kind === 'workspace-changed') {
      window.location.reload()
    } else if (message.kind === 'permissions-changed') {
      void refreshPermissions()
    } else {
      noteAlive()
    }
  })

  async function refreshPermissions() {
    if (authState.value !== 'signed-in') return
    try {
      const res = await authClient.getSession({})
      const before = [...(user.value?.systemPermissions ?? [])].sort().join(',')
      applySession(res.session ?? undefined)
      const after = [...(user.value?.systemPermissions ?? [])].sort().join(',')
      if (authState.value === 'signed-in' && before !== after) toast.info('Your access changed.')
    } catch (err) {
      // Unauthenticated is already handled globally by the interceptor.
      if (!(err instanceof ConnectError) || err.code !== Code.Unauthenticated) {
        toast.error(err instanceof ConnectError ? `Could not refresh your access: ${err.rawMessage}` : 'Could not refresh your access.')
      }
    }
  }

  setIdleSignOut(() => void signOut())
  startIdleTracking()

  // startDesk finds out what state the server is in and opens the right page. It
  // runs at start and again from "Check again" on the sign-in card.
  async function startDesk() {
    pageState.value = null
    // /healthz does not need the database, so it tells a server that is down
    // from a database that is down.
    try {
      const response = await fetch('/healthz')
      const health = await response.json() as { database?: string }
      databaseState.value = health.database ?? 'Not configured'
      apiState.value = 'connected'
    } catch {
      databaseState.value = 'Unavailable'
      apiState.value = 'error'
    }
    if (apiState.value === 'error') {
      pageState.value = { kind: 'unreachable', message: 'Cannot reach the Athena server. Check that it is running, then try again.', persistent: true }
      authState.value = 'signed-out'
      return
    }
    if (databaseState.value !== 'Connected') {
      const detail = databaseState.value === 'Not configured' ? 'No database is configured. Set ATHENA_DATABASE_URL and restart Athena.' : 'The database is not connected. Start MariaDB and check ATHENA_DATABASE_URL, then try again.'
      pageState.value = { kind: 'database', message: detail, persistent: true }
      authState.value = 'signed-out'
      return
    }
    try {
      const info = await systemClient.getSystemInfo({})
      workspaceName.value = info.workspaceName
      if (info.telemetryEnabled) startBrowserTelemetry(info.version, info.workspaceSlug)
      // The Workspace accent applies for every user, signed in or not. The
      // call is public, so the sign-in page wears it too. On failure the
      // cached first paint stands.
      try {
        const fetched = await workspaceClient.getAppearance({})
        if (fetched.appearance) {
          const palette = { light: fetched.lightAccent, dark: fetched.darkAccent }
          applyAppearance({ preset: fetched.appearance.preset, accent: fetched.appearance.accent, chartColors: [...fetched.appearance.chartColors] }, palette)
          saveCachedAppearance(info.workspaceSlug, fetched.appearance, palette)
        }
      } catch {
        // Keep the cache or the default; the page still works.
      }
    } catch (err) {
      apiState.value = 'error'
      pageState.value = classify(err)
      authState.value = 'signed-out'
      return
    }
    try {
      const res = await authClient.getSession({})
      hasUsers.value = res.hasUsers
      applySession(res.session)
      forceViewWithoutDesk()
      landOnBuildPage()
      if (authState.value === 'signed-in') await library.load()
    } catch (err) {
      const failure = classify(err)
      if (failure.persistent) pageState.value = failure
      else toast.error(describeFailure(failure))
      authState.value = 'signed-out'
    }
  }

  async function recheck() {
    rechecking.value = true
    try {
      await startDesk()
      if (pageState.value === null && !hasUsers.value) toast.error('Still no users. Run the command, then check again.')
    } finally {
      rechecking.value = false
    }
  }

  return {
    apiState, databaseState, workspaceName, authState, hasUsers, pageState,
    signInNote, rechecking, user, changingPassword,
    canBuild, canFiles, canWriteFiles, canRules, canWriteRules, canAccess,
    canLedger, canWriteLedger, canBuildDesk,
    signedIn, signOut, startDesk, recheck,
  }
}
