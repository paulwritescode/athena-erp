<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { authClient } from '../api'
import type { AccessEvent, AccessUser, Assignment, AssignmentExplanation, Role, RoleHolder, Session } from '../gen/athena/v1/auth_pb'
import type { RecordType } from '../types'
import { actionLegend, actionLetter, describeScope, diffAssignments, parseScope, previewRoleGrants } from '../assignmentPreview'
import { formatWhen, formatWhenFull } from '../times'
import { Button } from '@/components/ui/button'
import ActionButton from '@/components/ActionButton.vue'
import HelpHint from '@/copy/HelpHint.vue'
import ActionGroup from '@/components/ActionGroup.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import MultiSelectField from '@/components/fields/MultiSelectField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import type { ActionItem } from '@/components/actions'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'

// The Users & Roles page administers access through AuthService: inviting
// users, scoped dated assignments, sessions and the Permission Explorer.
// Every action is checked again on the server; the page only hides what the
// signed-in user may not do.
const props = defineProps<{ permissions: string[]; username: string; recordTypes: RecordType[] }>()

const canUsers = computed(() => props.permissions.includes('access:users'))
const canRoles = computed(() => props.permissions.includes('access:roles'))
const canAssign = computed(() => props.permissions.includes('access:assign'))
const canExplore = computed(() => props.permissions.includes('access:explore'))
const canAudit = computed(() => props.permissions.includes('audit'))

const tab = ref(canUsers.value ? 'users' : canRoles.value ? 'roles' : 'sessions')
const users = ref<AccessUser[]>([])
const roles = ref<Role[]>([])
const loaded = ref(false)
const unavailable = ref('')
const busy = ref(false)

function describe(err: unknown, action: string): string {
  if (!(err instanceof ConnectError)) return `Could not ${action}.`
  if (err.code === Code.PermissionDenied) return `You do not have permission to ${action}.`
  if (err.code === Code.Aborted) return 'This changed since you opened it. Reload it, then apply your edit again.'
  return `Could not ${action}: ${err.rawMessage}`
}

function stateVariant(state: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (state) {
    case 'active':
      return 'default'
    case 'invited':
      return 'secondary'
    case 'suspended':
      return 'destructive'
    default:
      return 'outline'
  }
}

function scopeText(assignment: Assignment): string {
  const scope: Record<string, string[]> = {}
  for (const [dimension, values] of Object.entries(assignment.scope)) scope[dimension] = [...values.values]
  return describeScope(scope)
}

async function refreshUsers() {
  if (!canUsers.value) return
  try {
    users.value = (await authClient.listUsers({})).users
  } catch (err) {
    toast.error(describe(err, 'load the users'))
  }
}

async function refreshRoles() {
  if (!canRoles.value) return
  try {
    roles.value = (await authClient.listRoles({})).roles
  } catch (err) {
    toast.error(describe(err, 'load the Roles'))
  }
}

async function refresh() {
  try {
    await Promise.all([refreshUsers(), refreshRoles()])
    unavailable.value = ''
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.Unavailable) unavailable.value = 'The Athena server is unavailable. Check that it is running, then try again.'
  } finally {
    loaded.value = true
  }
}

// The selected user and their assignments.
const selectedUser = ref('')
const assignments = ref<Assignment[]>([])
const assignmentsLoaded = ref(false)
// Supplementary sheet rows, best-effort: a failed fetch hides its row.
const sessionCount = ref<number | null>(null)
const userEvents = ref<AccessEvent[]>([])

// Each user selection gets a number so a slow response for a previously
// selected user never overwrites the current sheet.
let userRequest = 0

async function selectUser(username: string) {
  const request = ++userRequest
  selectedUser.value = username
  assignmentsLoaded.value = false
  sessionCount.value = null
  userEvents.value = []
  if (!username) {
    assignments.value = []
    return
  }
  try {
    const res = await authClient.listAssignments({ username })
    if (request !== userRequest) return
    assignments.value = res.assignments
  } catch (err) {
    if (request !== userRequest) return
    toast.error(describe(err, 'load the assignments'))
  } finally {
    if (request === userRequest) assignmentsLoaded.value = true
  }
  try {
    const res = await authClient.listSessions({ username })
    if (request !== userRequest) return
    sessionCount.value = res.sessions.length
  } catch {
    if (request === userRequest) sessionCount.value = null
  }
  if (canAudit.value) {
    try {
      const res = await authClient.listAccessLog({ pageSize: 50 })
      if (request !== userRequest) return
      userEvents.value = res.events.filter((event) => event.username === username).slice(0, 5)
    } catch {
      if (request === userRequest) userEvents.value = []
    }
  }
}

// The account state in one plain line.
function stateLine(user: AccessUser): string {
  if (user.locked) return 'Locked after too many failed sign-ins.'
  if (user.disabled) return 'Suspended, cannot sign in.'
  switch (user.state) {
    case 'invited':
      return 'Invited, has not set a password yet.'
    case 'reset_required':
      return 'A reset was started, a new password is needed.'
    default:
      return 'Active, can sign in.'
  }
}

// Access Log events in plain words (vocabulary from internal/auth/accesslog.go).
function describeAccessEvent(event: AccessEvent): string {
  switch (event.event) {
    case 'login':
      return event.outcome === 'ok' ? 'Signed in' : 'Sign-in failed'
    case 'login_failed':
      return event.detail === 'locked' ? 'Sign-in failed (account locked)' : 'Sign-in failed'
    case 'lockout':
      return 'Locked after too many tries'
    case 'logout':
      return 'Signed out'
    case 'denied':
      return event.recordTypeKey ? `Denied ${event.action} on ${event.recordTypeKey}` : `Denied ${event.procedure}`
    case 'password_changed':
      return 'Changed their password'
    case 'password_reset':
      return 'Password was reset'
    case 'user_created':
      return 'Account created'
    case 'user_updated':
      return 'Account updated'
    case 'role_saved':
      return 'Role saved'
    case 'role_deleted':
      return 'Role deleted'
    case 'invite_created':
      return 'Invite created'
    case 'invite_resent':
      return 'Invite resent'
    case 'reset_started':
      return 'Reset started'
    case 'challenge_issued':
      return 'Challenge issued'
    case 'challenge_completed':
      return 'Challenge completed'
    case 'challenge_failed':
      return 'Challenge failed'
    default:
      return event.outcome ? `${event.event} · ${event.outcome}` : event.event
  }
}

// Jump from a user sheet into their sessions.
function gotoSessions(username: string) {
  void selectUser('')
  sessionUser.value = username
  tab.value = 'sessions'
  void loadSessions()
}

const selected = computed(() => users.value.find((user) => user.username === selectedUser.value) ?? null)

// The workspace root account is fully visible here but never editable from
// the desk: invites, resets, suspension and assignments cannot touch it.
const rootUsername = 'administrator'
const isRootSelected = computed(() => selected.value?.username === rootUsername)

// The user sheet footer: Assign first, then the secondary pair, then the
// caution or its restore opposite. The root account gets no actions.
const userActions = computed<ActionItem[]>(() => {
  if (!selected.value || isRootSelected.value) return []
  const items: ActionItem[] = []
  if (canAssign.value) items.push({ id: 'assign', label: 'Assign Role', kind: 'primary', icon: 'plus', busy: busy.value })
  if (canUsers.value && selected.value.state === 'invited') items.push({ id: 'resend', label: 'Resend invite', kind: 'secondary', icon: 'send', busy: busy.value })
  if (canUsers.value && (selected.value.state === 'active' || selected.value.state === 'reset_required')) {
    items.push({ id: 'reset', label: 'Reset', kind: 'secondary', icon: 'key-round', busy: busy.value })
  }
  if (canUsers.value && !selected.value.disabled) items.push({ id: 'suspend', label: 'Suspend', kind: 'caution', icon: 'pause', busy: busy.value })
  if (canUsers.value && selected.value.disabled) items.push({ id: 'reactivate', label: 'Reactivate', kind: 'restore', icon: 'play', busy: busy.value })
  return items
})

function onUserAction(id: string) {
  if (!selected.value) return
  if (id === 'assign') openNewAssignment()
  else if (id === 'resend') void resend(selected.value.username)
  else if (id === 'reset') resetting.value = true
  else if (id === 'suspend') suspending.value = true
  else if (id === 'reactivate') void reactivate()
}

// Field names across Record Types feed the Scope dimension suggestions: a
// dimension only ever matches a field of the same name.
const scopeDimensions = computed(() => {
  const names = new Set<string>()
  for (const recordType of props.recordTypes) for (const field of recordType.fields) names.add(field.name)
  return [...names].sort()
})

// Inviting. The Challenge is shown once in its own dialog.
const inviting = ref(false)
const inviteForm = ref({ username: '', displayName: '', roles: [] as string[] })
const inviteProblem = ref('')

// Dirty snapshots: closing a dialog with unsaved typing asks first.
const inviteSnapshot = ref('')
watch(inviting, (open) => {
  if (open) inviteSnapshot.value = JSON.stringify(inviteForm.value)
})
const inviteDirty = computed(() => inviting.value && JSON.stringify(inviteForm.value) !== inviteSnapshot.value)

const challengeOpen = ref(false)
const challengeTitle = ref('')
const challengeValue = ref('')

function showChallenge(title: string, challenge: string) {
  challengeTitle.value = title
  challengeValue.value = challenge
  challengeOpen.value = true
}

async function copyChallenge() {
  try {
    await navigator.clipboard.writeText(challengeValue.value)
    toast.success('Challenge copied.')
  } catch {
    toast.error('Could not copy. Select the challenge and copy it.')
  }
}

async function invite() {
  inviteProblem.value = ''
  const username = inviteForm.value.username.trim()
  const names = [...inviteForm.value.roles]
  for (const name of names) {
    if (!roles.value.some((role) => role.name === name)) {
      inviteProblem.value = `No Role named ${name}.`
      return
    }
  }
  if (!username) {
    inviteProblem.value = 'Enter a username.'
    return
  }
  busy.value = true
  try {
    const res = await authClient.inviteUser({ username, displayName: inviteForm.value.displayName.trim(), roles: names })
    inviting.value = false
    inviteForm.value = { username: '', displayName: '', roles: [] }
    await refreshUsers()
    await selectUser(res.user?.username ?? '')
    if (res.challenge) showChallenge(`Invite for ${res.user?.username}`, res.challenge)
    toast.success(`Invited ${res.user?.username}.`)
  } catch (err) {
    toast.error(describe(err, 'invite the user'))
  } finally {
    busy.value = false
  }
}

async function resend(username: string) {
  busy.value = true
  try {
    const res = await authClient.resendInvite({ username })
    await refreshUsers()
    if (res.challenge) showChallenge(`New invite for ${username}`, res.challenge)
    toast.success(`Resent the invite for ${username}.`)
  } catch (err) {
    toast.error(describe(err, 'resend the invite'))
  } finally {
    busy.value = false
  }
}

// Resetting needs the administrator's own password, then shows the reset
// Challenge once.
const resetting = ref(false)
const actorPassword = ref('')
const resetProblem = ref('')
const resetDirty = computed(() => resetting.value && actorPassword.value !== '')

async function startReset() {
  resetProblem.value = ''
  if (!actorPassword.value) {
    resetProblem.value = 'Enter your own password.'
    return
  }
  busy.value = true
  try {
    const res = await authClient.startReset({ username: selectedUser.value, actorPassword: actorPassword.value })
    actorPassword.value = ''
    resetting.value = false
    await refreshUsers()
    if (res.challenge) showChallenge(`Reset for ${selectedUser.value}`, res.challenge)
    toast.success(`Started a reset for ${selectedUser.value}.`)
  } catch (err) {
    actorPassword.value = ''
    toast.error(describe(err, 'start the reset'))
  } finally {
    busy.value = false
  }
}

const suspending = ref(false)

async function suspend() {
  suspending.value = false
  busy.value = true
  try {
    await authClient.updateUser({ username: selectedUser.value, disabled: true })
    toast.success(`Suspended ${selectedUser.value}.`)
    await refreshUsers()
  } catch (err) {
    toast.error(describe(err, 'suspend the user'))
  } finally {
    busy.value = false
  }
}

async function reactivate() {
  busy.value = true
  try {
    await authClient.updateUser({ username: selectedUser.value, disabled: false })
    toast.success(`Reactivated ${selectedUser.value}. Review their privileged assignments.`)
    await refreshUsers()
  } catch (err) {
    toast.error(describe(err, 'reactivate the user'))
  } finally {
    busy.value = false
  }
}

// Assignments: one Role with Scope limits and dates, edited row by row so no
// syntax shows. Saving runs through a preview dialog with the diff, and
// edits carry the expected revision.
type ScopeRow = { dimension: string; values: string }
type AssignmentForm = { id: string; role: string; rows: ScopeRow[]; activeFrom: string; expiresMode: 'never' | 'date'; expiresAt: string; revision: bigint }
const blankAssignment = (): AssignmentForm => ({ id: '', role: '', rows: [], activeFrom: '', expiresMode: 'never', expiresAt: '', revision: BigInt(0) })

const assignmentOpen = ref(false)
const assignmentForm = ref<AssignmentForm>(blankAssignment())
const assignmentProblem = ref('')
const assignmentRevision = ref<bigint>(BigInt(0))
const assignmentSnapshot = ref('')
watch(assignmentOpen, (open) => {
  if (open) assignmentSnapshot.value = JSON.stringify(assignmentForm.value, (_, value) => (typeof value === 'bigint' ? String(value) : value))
})
const assignmentDirty = computed(
  () =>
    assignmentOpen.value &&
    JSON.stringify(assignmentForm.value, (_, value) => (typeof value === 'bigint' ? String(value) : value)) !== assignmentSnapshot.value,
)

// formScope validates every row through the shared parser and reports the
// first problem, if any.
function formScope(): { scope: Record<string, string[]>; problem: string } {
  const scope: Record<string, string[]> = {}
  for (const row of assignmentForm.value.rows) {
    const parsed = parseScope(`${row.dimension}=${row.values}`)
    if (parsed.problems.length > 0) return { scope: {}, problem: parsed.problems[0] ?? 'A Scope limit does not parse.' }
    for (const [dimension, values] of Object.entries(parsed.scope)) {
      if (scope[dimension] !== undefined) return { scope: {}, problem: `Two limits name ${dimension}.` }
      scope[dimension] = values
    }
  }
  return { scope, problem: '' }
}

function rowProblem(row: ScopeRow): string {
  if (!row.dimension.trim() && !row.values.trim()) return ''
  return parseScope(`${row.dimension}=${row.values}`).problems[0] ?? ''
}

function toForm(assignment: Assignment): AssignmentForm {
  const rows: ScopeRow[] = Object.entries(assignment.scope)
    .sort(([a], [b]) => (a < b ? -1 : 1))
    .map(([dimension, values]) => ({ dimension, values: [...values.values].join(', ') }))
  return {
    id: assignment.id,
    role: assignment.role,
    rows,
    activeFrom: assignment.activeFrom,
    expiresMode: assignment.expiresAt ? 'date' : 'never',
    expiresAt: assignment.expiresAt,
    revision: assignment.revision,
  }
}

function openNewAssignment() {
  assignmentForm.value = blankAssignment()
  assignmentRevision.value = BigInt(0)
  assignmentProblem.value = ''
  assignmentOpen.value = true
}

function openEditAssignment(assignment: Assignment) {
  assignmentForm.value = toForm(assignment)
  assignmentRevision.value = assignment.revision
  assignmentProblem.value = ''
  assignmentOpen.value = true
}

function addScopeRow() {
  assignmentForm.value.rows.push({ dimension: '', values: '' })
}

function removeScopeRow(index: number) {
  assignmentForm.value.rows.splice(index, 1)
}

function assignmentActions(): ActionItem[] {
  return [
    { id: 'edit', label: 'Edit', kind: 'secondary', icon: 'pencil', busy: busy.value },
    { id: 'revoke', label: 'Revoke', kind: 'caution', icon: 'undo-2', busy: busy.value },
  ]
}

function onAssignmentAction(assignment: Assignment, id: string) {
  if (id === 'edit') openEditAssignment(assignment)
  else revoking.value = assignment
}

const previewOpen = ref(false)
const previewLines = ref<string[]>([])
const previewGrants = ref<string[]>([])

function openPreview() {
  assignmentProblem.value = ''
  if (!assignmentForm.value.role) {
    assignmentProblem.value = 'Choose a Role.'
    return
  }
  const { scope, problem } = formScope()
  if (problem) {
    assignmentProblem.value = problem
    return
  }
  const expiresAt = assignmentForm.value.expiresMode === 'date' ? assignmentForm.value.expiresAt : ''
  if (assignmentForm.value.expiresMode === 'date' && !expiresAt) {
    assignmentProblem.value = 'Enter the expiry date or choose Never expires.'
    return
  }
  const next = { role: assignmentForm.value.role, scope, activeFrom: assignmentForm.value.activeFrom, expiresAt }
  if (assignmentForm.value.id) {
    const current = assignments.value.find((item) => item.id === assignmentForm.value.id)
    if (!current) {
      assignmentProblem.value = 'The Assignment is gone. Reload it.'
      return
    }
    previewLines.value = diffAssignments(
      { role: current.role, scope: scopeOf(current), activeFrom: current.activeFrom, expiresAt: current.expiresAt },
      next,
    )
  } else {
    previewLines.value = diffAssignments({ role: '', scope: {}, activeFrom: '', expiresAt: '' }, next)
  }
  const role = roles.value.find((item) => item.name === assignmentForm.value.role)
  const labels: Record<string, string> = {}
  for (const recordType of props.recordTypes) labels[recordType.name] = recordType.label || recordType.name
  previewGrants.value = role ? previewRoleGrants(role.grants, labels, scope) : []
  previewOpen.value = true
}

function scopeOf(assignment: Assignment): Record<string, string[]> {
  const scope: Record<string, string[]> = {}
  for (const [dimension, values] of Object.entries(assignment.scope)) scope[dimension] = [...values.values]
  return scope
}

async function confirmSaveAssignment() {
  previewOpen.value = false
  busy.value = true
  try {
    const { scope } = formScope()
    const message: Record<string, { values: string[] }> = {}
    for (const [dimension, values] of Object.entries(scope)) message[dimension] = { values: [...values] }
    await authClient.saveAssignment({
      username: selectedUser.value,
      assignment: {
        id: assignmentForm.value.id,
        role: assignmentForm.value.role,
        scope: message,
        activeFrom: assignmentForm.value.activeFrom,
        expiresAt: assignmentForm.value.expiresMode === 'date' ? assignmentForm.value.expiresAt : '',
        revision: assignmentRevision.value,
      },
    })
    assignmentOpen.value = false
    toast.success(assignmentForm.value.id ? 'Saved the Assignment.' : 'Assigned the Role.')
    await selectUser(selectedUser.value)
  } catch (err) {
    toast.error(describe(err, 'save the Assignment'))
    await selectUser(selectedUser.value)
  } finally {
    busy.value = false
  }
}

const revoking = ref<Assignment | null>(null)

async function revoke() {
  const target = revoking.value
  revoking.value = null
  if (!target) return
  busy.value = true
  try {
    await authClient.revokeAssignment({ id: target.id })
    toast.success(`Revoked ${target.role}.`)
    await selectUser(selectedUser.value)
  } catch (err) {
    toast.error(describe(err, 'revoke the Assignment'))
  } finally {
    busy.value = false
  }
}

// Roles tab: the list with one Role's Grants, plus creating, renaming
// descriptions and permissions, and deleting. Grants themselves are still
// saved from the command line; editing a Role keeps its Grants untouched.
const selectedRole = ref('')
const roleDetail = computed(() => roles.value.find((role) => role.name === selectedRole.value) ?? null)

const roleActions = computed<ActionItem[]>(() => {
  if (!roleDetail.value) return []
  return [
    { id: 'edit', label: 'Edit', kind: 'secondary', icon: 'pencil', busy: busy.value },
    { id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2', busy: busy.value },
  ]
})

function onRoleAction(id: string) {
  if (!roleDetail.value) return
  if (id === 'edit') openEditRole(roleDetail.value)
  else if (id === 'delete') deletingRole.value = roleDetail.value
}

const systemPermissionOptions = [
  'build',
  'audit',
  'files:read',
  'files:write',
  'rules:read',
  'rules:write',
  'access:ownership',
  'access:users',
  'access:roles',
  'access:assign',
  'access:recover',
  'access:sessions',
  'access:explore',
]
const roleDialogOpen = ref(false)
const roleEditing = ref(false)
const roleForm = ref({ name: '', description: '', permissions: [] as string[] })
const roleProblem = ref('')
const roleSnapshot = ref('')
watch(roleDialogOpen, (open) => {
  if (open) roleSnapshot.value = JSON.stringify(roleForm.value)
})
const roleDirty = computed(() => roleDialogOpen.value && JSON.stringify(roleForm.value) !== roleSnapshot.value)

function openNewRole() {
  roleEditing.value = false
  roleForm.value = { name: '', description: '', permissions: [] }
  roleProblem.value = ''
  roleDialogOpen.value = true
}

function openEditRole(role: Role) {
  roleEditing.value = true
  roleForm.value = { name: role.name, description: role.description, permissions: [...role.systemPermissions] }
  roleProblem.value = ''
  roleDialogOpen.value = true
}

async function saveRole() {
  roleProblem.value = ''
  if (!roleForm.value.name.trim()) {
    roleProblem.value = 'Enter a Role name.'
    return
  }
  busy.value = true
  try {
    const grants = roleEditing.value ? (roles.value.find((role) => role.name === roleForm.value.name)?.grants ?? []) : []
    await authClient.saveRole({
      role: {
        name: roleForm.value.name.trim(),
        description: roleForm.value.description.trim(),
        systemPermissions: [...roleForm.value.permissions],
        grants: grants.map((grant) => ({
          recordTypeKey: grant.recordTypeKey,
          actions: [...grant.actions],
          permissionLevel: grant.permissionLevel,
          condition: grant.condition,
          separateDuties: grant.separateDuties,
        })),
      },
    })
    roleDialogOpen.value = false
    toast.success(roleEditing.value ? `Saved ${roleForm.value.name}.` : `Created ${roleForm.value.name}.`)
    await refreshRoles()
  } catch (err) {
    toast.error(describe(err, 'save the Role'))
  } finally {
    busy.value = false
  }
}

const deletingRole = ref<Role | null>(null)

async function deleteRole() {
  const target = deletingRole.value
  deletingRole.value = null
  if (!target) return
  busy.value = true
  try {
    await authClient.deleteRole({ name: target.name })
    if (selectedRole.value === target.name) selectedRole.value = ''
    toast.success(`Deleted ${target.name}.`)
    await refreshRoles()
  } catch (err) {
    toast.error(describe(err, 'delete the Role'))
  } finally {
    busy.value = false
  }
}

function toggleRolePermission(permission: string, checked: boolean | 'indeterminate') {
  const selected = new Set(roleForm.value.permissions)
  if (checked === true) selected.add(permission)
  else selected.delete(permission)
  roleForm.value.permissions = [...selected]
}

// Permissions tab: who sees what. Rows come from the Grants themselves, so
// the matrix works without the build permission. A type row shows effective
// actions (its own Grants plus *-Grants); the All-record-types row shows the
// *-Grants alone. Each letter opens the holders of that action.
const matrixTypes = computed(() => {
  const keys = new Set<string>()
  for (const role of roles.value) for (const grant of role.grants) keys.add(grant.recordTypeKey)
  const rest = [...keys].filter((key) => key !== '*').sort()
  return keys.has('*') ? ['*', ...rest] : rest
})

function cellActions(role: Role, typeKey: string): string[] {
  const held = new Set<string>()
  for (const grant of role.grants) {
    if (grant.recordTypeKey !== typeKey && (typeKey === '*' || grant.recordTypeKey !== '*')) continue
    for (const action of grant.actions) held.add(action)
  }
  return grantActions.filter((action) => held.has(action))
}

const holdersOpen = ref(false)
const holdersTitle = ref({ role: '', typeKey: '', action: '' })
const holders = ref<RoleHolder[]>([])

let holdersRequest = 0

async function openHolders(role: string, typeKey: string, action: string) {
  const request = ++holdersRequest
  busy.value = true
  try {
    const res = await authClient.listRoleHolders({ role, recordTypeKey: typeKey, action })
    if (request !== holdersRequest) return
    holders.value = res.holders
    holdersTitle.value = { role, typeKey, action }
    holdersOpen.value = true
  } catch (err) {
    if (request !== holdersRequest) return
    toast.error(describe(err, 'load the holders'))
  } finally {
    if (request === holdersRequest) busy.value = false
  }
}

function holderScope(holder: RoleHolder): string {
  const scope: Record<string, string[]> = {}
  for (const [dimension, values] of Object.entries(holder.scope)) scope[dimension] = [...values.values]
  return describeScope(scope)
}

// Jump from a holder into the Explorer, prefilled. The *-row carries no
// concrete type, so it fills in and waits instead of running.
function explainHolder(holder: RoleHolder) {
  exploreForm.value = { username: holder.username, action: holdersTitle.value.action, recordType: holdersTitle.value.typeKey === '*' ? '' : holdersTitle.value.typeKey, recordId: '' }
  holdersOpen.value = false
  tab.value = 'explore'
  if (exploreForm.value.recordType) void explain()
}

// Sessions tab. Every user lists their own; another user's need the
// sessions permission, which the server checks.
const sessionUser = ref(props.username)
const loadedSessionUser = ref('')
const sessions = ref<Session[]>([])
const selectedSession = ref('')
const sessionDetail = computed(() => sessions.value.find((session) => session.id === selectedSession.value) ?? null)

const sessionActions = computed<ActionItem[]>(() => {
  if (!sessionDetail.value) return []
  return [{ id: 'end', label: 'End session', kind: 'caution', icon: 'log-out', busy: busy.value }]
})

function onSessionAction(id: string) {
  if (id === 'end' && sessionDetail.value) endingSession.value = sessionDetail.value
}

async function loadSessions() {
  const username = sessionUser.value.trim()
  if (!username) return
  busy.value = true
  try {
    const res = await authClient.listSessions({ username })
    if (sessionUser.value.trim() !== username) return
    sessions.value = res.sessions
    loadedSessionUser.value = username
  } catch (err) {
    if (sessionUser.value.trim() !== username) return
    toast.error(describe(err, 'load the sessions'))
  } finally {
    if (sessionUser.value.trim() === username) busy.value = false
  }
}

const endingSession = ref<Session | null>(null)

async function revokeSession(id: string) {
  busy.value = true
  try {
    await authClient.revokeSession({ id })
    toast.success('Ended the session.')
    await loadSessions()
  } catch (err) {
    toast.error(describe(err, 'end the session'))
  } finally {
    busy.value = false
  }
}

function confirmEndSession() {
  const session = endingSession.value
  endingSession.value = null
  if (session) void revokeSession(session.id)
}

// Permission Explorer. With a Record the verdicts test row conditions and
// Scopes against it; without one they cover dates, required Scope and
// Grants only.
const grantActions = ['read', 'create', 'write', 'delete', 'submit', 'cancel', 'amend', 'audit']
const exploreForm = ref({ username: props.username, action: 'read', recordType: '', recordId: '' })
const explanation = ref<{ allowed: boolean; assignments: AssignmentExplanation[] } | null>(null)

let explainRequest = 0

async function explain() {
  if (!exploreForm.value.username.trim() || !exploreForm.value.recordType.trim()) {
    toast.error('Enter a username and a Record Type.')
    return
  }
  const request = ++explainRequest
  busy.value = true
  try {
    const res = await authClient.explainAccess({
      username: exploreForm.value.username.trim(),
      action: exploreForm.value.action,
      recordTypeKey: exploreForm.value.recordType.trim(),
      recordId: exploreForm.value.recordId.trim(),
    })
    if (request !== explainRequest) return
    explanation.value = res
  } catch (err) {
    if (request !== explainRequest) return
    explanation.value = null
    toast.error(describe(err, 'explain the access'))
  } finally {
    if (request === explainRequest) busy.value = false
  }
}

function onTab(value: string | number) {
  tab.value = String(value)
  if (tab.value === 'sessions' && sessions.value.length === 0) void loadSessions()
}

onMounted(async () => {
  await refresh()
})
</script>

<template>
  <section class="page-content files-page">
    <div class="page-title-row">
      <div><h1>Access</h1><p class="lead">Invite users, give them Roles, and see who can do what.</p></div>
      <div v-if="tab === 'users' && canUsers && !unavailable" class="toolbar-actions"><ActionButton kind="primary" label="Invite user" icon="plus" :busy="busy" @press="inviting = true" /></div>
    </div>
    <div v-if="unavailable" class="empty-builder"><div class="card-icon">⌘</div><h2>Access is unavailable</h2><p>{{ unavailable }}</p></div>
    <Tabs v-else :model-value="tab" @update:model-value="onTab">
      <TabsList>
        <TabsTrigger v-if="canUsers" value="users">Users</TabsTrigger>
        <TabsTrigger v-if="canRoles" value="roles">Roles</TabsTrigger>
        <TabsTrigger v-if="canRoles" value="permissions">Permissions</TabsTrigger>
        <TabsTrigger value="sessions">Sessions</TabsTrigger>
        <TabsTrigger v-if="canExplore" value="explore">Explore</TabsTrigger>
      </TabsList>
      <TabsContent value="users">
        <p v-if="loaded && users.length === 0" class="files-empty">No users yet.</p>
        <Table v-else class="datatable-stacked">
              <TableHeader><TableRow><TableHead>Username</TableHead><TableHead>Display name</TableHead><TableHead>State</TableHead><TableHead>Roles</TableHead><TableHead>Last sign-in</TableHead></TableRow></TableHeader>
              <TableBody>
                <TableRow v-for="user in users" :key="user.username" class="cursor-pointer" :data-state="user.username === selectedUser ? 'selected' : undefined" @click="selectUser(user.username === selectedUser ? '' : user.username)">
                  <TableCell data-label="Username"><button type="button" class="row-open" :aria-expanded="user.username === selectedUser" :aria-label="`Open details for ${user.username}`" @click.stop="selectUser(user.username === selectedUser ? '' : user.username)">{{ user.username }}</button> <Badge v-if="user.owner" variant="secondary">Owner</Badge></TableCell>
                  <TableCell data-label="Display name">{{ user.displayName }}</TableCell>
                  <TableCell data-label="State"><Badge :variant="stateVariant(user.state)">{{ user.state || 'active' }}</Badge><Badge v-if="user.locked" variant="destructive">Locked</Badge></TableCell>
                  <TableCell data-label="Roles" class="datatable-hide-mobile"><span v-if="user.roles.length > 0">{{ user.roles.join(', ') }}</span><span v-else-if="user.username === rootUsername" class="muted-label">Full access</span><span v-else class="muted-label">No roles</span></TableCell>
                  <TableCell data-label="Last sign-in" class="datatable-hide-mobile" :title="formatWhenFull(user.lastLoginAt)">{{ formatWhen(user.lastLoginAt) }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
        <DetailSheet
          v-if="selected"
          :open="true"
          :title="selected.displayName || selected.username"
          wide
          @update:open="(value: boolean) => { if (!value) selectUser('') }"
        >
          <template #status>
            <Badge :variant="stateVariant(selected.state)">{{ selected.state || 'active' }}</Badge>
            <Badge v-if="selected.locked" variant="destructive">Locked</Badge>
          </template>
          <template #subtitle>
            <p class="text-caption text-muted-foreground font-mono">{{ selected.username }}</p>
          </template>
          <p class="muted-label">{{ stateLine(selected) }}</p>
          <p v-if="selected.lastLoginAt" class="muted-label" :title="formatWhenFull(selected.lastLoginAt)">Last sign-in {{ formatWhen(selected.lastLoginAt) }}</p>
          <p v-if="isRootSelected" class="editor-footnote">The root account is managed outside the desk and cannot be changed here.</p>
          <div class="sheet-section">
            <p class="sheet-label">Roles</p>
            <p v-if="assignmentsLoaded && assignments.length === 0" class="files-empty">No assignments. {{ canAssign && !isRootSelected ? 'Assign a Role.' : '' }}</p>
            <ul v-else class="access-assignments">
              <li v-for="assignment in assignments" :key="assignment.id" class="access-assignment">
                <Badge>{{ assignment.role }}</Badge>
                <span class="muted-label">{{ scopeText(assignment) }}</span>
                <span v-if="assignment.activeFrom || assignment.expiresAt" class="muted-label">{{ assignment.activeFrom || '…' }} → {{ assignment.expiresAt || '…' }}</span>
                <ActionGroup
                  v-if="canAssign && !isRootSelected"
                  :actions="assignmentActions()"
                  density="row"
                  class="access-assignment-actions"
                  :aria-label="`Actions for ${assignment.role}`"
                  @press="(id: string) => onAssignmentAction(assignment, id)"
                />
              </li>
            </ul>
          </div>
          <div v-if="sessionCount !== null" class="sheet-section">
            <p class="sheet-label">Sessions</p>
            <div class="sheet-inline">
              <span class="muted-label">{{ sessionCount === 0 ? 'No active sessions.' : `${sessionCount} active ${sessionCount === 1 ? 'session' : 'sessions'}.` }}</span>
              <ActionButton v-if="sessionCount > 0" kind="quiet" density="inline" label="View in Sessions" icon="eye" @press="gotoSessions(selected.username)" />
            </div>
          </div>
          <div v-if="userEvents.length > 0" class="sheet-section">
            <p class="sheet-label">Recent events</p>
            <ul class="sheet-events">
              <li v-for="event in userEvents" :key="String(event.id)">
                <span>{{ describeAccessEvent(event) }}</span>
                <span class="muted-label">{{ formatWhenFull(event.at) }}</span>
              </li>
            </ul>
          </div>
          <template v-if="userActions.length > 0" #footer>
            <ActionGroup :actions="userActions" stack-on-phones aria-label="User actions" @press="onUserAction" />
          </template>
        </DetailSheet>
      </TabsContent>
      <TabsContent value="roles">
        <div class="toolbar-actions access-tab-actions"><ActionButton kind="primary" density="row" label="New Role" icon="plus" :busy="busy" @press="openNewRole" /></div>
        <p v-if="loaded && roles.length === 0" class="files-empty">No Roles yet.</p>
        <Table v-else class="datatable-stacked">
              <TableHeader><TableRow><TableHead>Role</TableHead><TableHead>Description</TableHead><TableHead>System permissions</TableHead><TableHead>Grants <HelpHint term="grant" /></TableHead></TableRow></TableHeader>
              <TableBody>
                <TableRow v-for="role in roles" :key="role.name" class="cursor-pointer" :data-state="role.name === selectedRole ? 'selected' : undefined" @click="selectedRole = role.name === selectedRole ? '' : role.name">
                  <TableCell data-label="Role"><button type="button" class="row-open" :aria-expanded="role.name === selectedRole" :aria-label="`Open details for ${role.name}`" @click.stop="selectedRole = role.name === selectedRole ? '' : role.name">{{ role.name }}</button></TableCell>
                  <TableCell data-label="Description" class="datatable-hide-mobile">{{ role.description }}</TableCell>
                  <TableCell data-label="System permissions" class="datatable-hide-mobile">{{ role.systemPermissions.join(', ') }}</TableCell>
                  <TableCell data-label="Grants">{{ role.grants.length }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
        <DetailSheet
          v-if="roleDetail"
          :open="true"
          :title="roleDetail.name"
          wide
          @update:open="(value: boolean) => { if (!value) selectedRole = '' }"
        >
          <p class="sheet-lead">{{ roleDetail.description || 'No description.' }}</p>
          <div class="sheet-section">
            <p class="sheet-label">System permissions</p>
            <p class="muted-label">{{ roleDetail.systemPermissions.length > 0 ? roleDetail.systemPermissions.join(', ') : 'None.' }}</p>
          </div>
          <div class="sheet-section">
            <p class="sheet-label">Grants</p>
            <p v-if="roleDetail.grants.length === 0" class="files-empty">No Grants. Roles gain Grants from the command line.</p>
            <ul v-else class="access-grants">
              <li v-for="(grant, index) in roleDetail.grants" :key="index">
                <strong>{{ grant.actions.join(', ') }}</strong> {{ grant.recordTypeKey === '*' ? 'everything' : grant.recordTypeKey }}
                <span v-if="grant.condition" class="muted-label">where {{ grant.condition }}</span>
                <Badge v-if="grant.separateDuties" variant="outline">Separate duties</Badge>
              </li>
            </ul>
          </div>
          <template #footer>
            <ActionGroup :actions="roleActions" stack-on-phones aria-label="Role actions" @press="onRoleAction" />
          </template>
        </DetailSheet>
      </TabsContent>
      <TabsContent value="permissions">
        <p v-if="loaded && roles.length === 0" class="files-empty">No Roles yet.</p>
        <Table v-else class="datatable-stacked">
          <TableHeader><TableRow><TableHead>Record Type <HelpHint term="recordType" /></TableHead><TableHead v-for="role in roles" :key="role.name">{{ role.name }}</TableHead></TableRow></TableHeader>
          <TableBody>
            <TableRow v-for="typeKey in matrixTypes" :key="typeKey">
              <TableCell data-label="Record Type">{{ typeKey === '*' ? 'All record types' : typeKey }}</TableCell>
              <TableCell v-for="role in roles" :key="role.name" :data-label="role.name" :class="cellActions(role, typeKey).length === 0 ? 'datatable-hide-mobile' : undefined">
                <span v-if="cellActions(role, typeKey).length === 0" class="muted-label">·</span>
                <ActionButton v-for="action in cellActions(role, typeKey)" :key="action" kind="quiet" density="row" :label="actionLetter(action)" :aria-label="`${action} ${typeKey === '*' ? 'everything' : typeKey} — show holders`" :title="`${action} ${typeKey === '*' ? 'everything' : typeKey} — show holders`" :disabled-reason="busy ? 'Please wait.' : null" class="access-letter" @press="openHolders(role.name, typeKey, action)" />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p class="editor-footnote">Letters open the holders of that action. {{ actionLegend.map(([letter, action]) => `${letter} ${action}`).join(' · ') }}.</p>
      </TabsContent>
      <TabsContent value="sessions">
        <div class="editor-toolbar">
          <div class="settings-field session-user">
            <Label for="session-user">User</Label>
            <Input id="session-user" v-model="sessionUser" autocomplete="off" :disabled="busy" @keyup.enter="loadSessions" />
          </div>
          <div class="toolbar-actions"><ActionButton kind="secondary" label="List sessions" icon="refresh-ccw" :busy="busy" @press="loadSessions" /></div>
        </div>
        <p v-if="sessions.length === 0" class="files-empty">No live sessions.</p>
        <Table v-else class="datatable-stacked">
              <TableHeader><TableRow><TableHead>Started</TableHead><TableHead>Last seen</TableHead><TableHead>Expires</TableHead><TableHead>Device</TableHead><TableHead></TableHead><TableHead></TableHead></TableRow></TableHeader>
              <TableBody>
                <TableRow v-for="session in sessions" :key="session.id" class="cursor-pointer" :data-state="session.id === selectedSession ? 'selected' : undefined" @click="selectedSession = session.id === selectedSession ? '' : session.id">
                  <TableCell data-label="Started" class="datatable-hide-mobile" :title="formatWhenFull(session.createdAt)">{{ formatWhen(session.createdAt) }}</TableCell>
                  <TableCell data-label="Last seen" :title="formatWhenFull(session.lastSeenAt)">{{ formatWhen(session.lastSeenAt) }}</TableCell>
                  <TableCell data-label="Expires" class="datatable-hide-mobile" :title="formatWhenFull(session.expiresAt)">{{ formatWhen(session.expiresAt) }}</TableCell>
                  <TableCell data-label="Device"><button type="button" class="row-open" :aria-expanded="session.id === selectedSession" :aria-label="`Open details for session ${session.id.slice(0, 8)}`" @click.stop="selectedSession = session.id === selectedSession ? '' : session.id">{{ session.device || '—' }}</button></TableCell>
                  <TableCell data-label="Status" :class="!session.current ? 'datatable-hide-mobile' : undefined"><Badge v-if="session.current" variant="secondary">This device</Badge></TableCell>
                  <TableCell><span @click.stop="() => {}"><ActionButton kind="caution" density="row" label="End" icon="log-out" :disabled-reason="busy ? 'Please wait.' : null" class="max-sm:hidden" @press="endingSession = session" /></span></TableCell>
                </TableRow>
              </TableBody>
            </Table>
        <DetailSheet
          v-if="sessionDetail"
          :open="true"
          :title="sessionDetail.device || 'Session'"
          @update:open="(value: boolean) => { if (!value) selectedSession = '' }"
        >
          <template #status>
            <Badge v-if="sessionDetail.current" variant="secondary">This device</Badge>
          </template>
          <p class="sheet-lead">Signed in as {{ loadedSessionUser }}</p>
          <p class="muted-label">Session <code>{{ sessionDetail.id.slice(0, 8) }}</code></p>
          <p class="muted-label" :title="formatWhenFull(sessionDetail.createdAt)">Started {{ formatWhen(sessionDetail.createdAt) }}</p>
          <p class="muted-label" :title="formatWhenFull(sessionDetail.lastSeenAt)">Last seen {{ formatWhen(sessionDetail.lastSeenAt) }}</p>
          <p class="muted-label" :title="formatWhenFull(sessionDetail.expiresAt)">Expires {{ formatWhen(sessionDetail.expiresAt) }}</p>
          <template #footer>
            <ActionGroup :actions="sessionActions" stack-on-phones aria-label="Session actions" @press="onSessionAction" />
          </template>
        </DetailSheet>
      </TabsContent>
      <TabsContent value="explore">
        <div class="editor-toolbar explore-form">
          <div class="settings-field"><Label for="explore-user">Username</Label><Input id="explore-user" v-model="exploreForm.username" autocomplete="off" :disabled="busy" /></div>
          <div class="settings-field"><Label for="explore-action">Action</Label>
            <Select v-model="exploreForm.action" :disabled="busy">
              <SelectTrigger id="explore-action"><SelectValue placeholder="Action" /></SelectTrigger>
              <SelectContent><SelectItem v-for="action in grantActions" :key="action" :value="action">{{ action }}</SelectItem></SelectContent>
            </Select>
          </div>
          <div class="settings-field"><Label for="explore-type">Record Type</Label><Input id="explore-type" v-model="exploreForm.recordType" autocomplete="off" spellcheck="false" :disabled="busy" /></div>
          <div class="settings-field"><Label for="explore-record">Record ID (optional)</Label><Input id="explore-record" v-model="exploreForm.recordId" autocomplete="off" spellcheck="false" :disabled="busy" /></div>
          <div class="toolbar-actions"><ActionButton kind="primary" label="Explain" icon="circle-help" :busy="busy" @press="explain" /></div>
        </div>
        <div v-if="explanation" class="record-editor files-editor-card">
          <div class="record-editor-head">
            <h2 class="files-title">{{ exploreForm.username }} {{ explanation.allowed ? 'may' : 'may not' }} {{ exploreForm.action }} {{ exploreForm.recordType }}</h2>
            <Badge :variant="explanation.allowed ? 'default' : 'destructive'">{{ explanation.allowed ? 'Allowed' : 'Denied' }}</Badge>
          </div>
          <ul class="access-verdicts">
            <li v-for="verdict in explanation.assignments" :key="verdict.assignmentId">
              <Badge>{{ verdict.role }}</Badge>
              <Badge :variant="verdict.matched ? 'default' : 'outline'">{{ verdict.matched ? 'Matches' : 'Fails' }}</Badge>
              <span v-for="reason in verdict.reasons" :key="reason" class="muted-label">{{ reason }}</span>
              <Badge v-if="verdict.separateDutiesConflict" variant="outline">Separate duties conflict</Badge>
            </li>
          </ul>
          <p v-if="!exploreForm.recordId" class="editor-footnote">Without a Record the verdicts cover dates, required Scope and Grants only; row conditions and Scopes stay untested.</p>
        </div>
      </TabsContent>
    </Tabs>

    <ResponsiveDialog v-model:open="inviting" title="Invite user" description="The user signs in for the first time with a one-time code, shown once." :dirty="inviteDirty">
        <div class="settings-field"><Label for="invite-username">Username</Label><Input id="invite-username" v-model="inviteForm.username" autocomplete="off" spellcheck="false" :disabled="busy" /></div>
        <div class="settings-field"><Label for="invite-display">Display name</Label><Input id="invite-display" v-model="inviteForm.displayName" autocomplete="off" :disabled="busy" /></div>
        <MultiSelectField
          id="invite-roles"
          label="Roles"
          optional
          v-model="inviteForm.roles"
          placeholder="Add Roles…"
          :help="roles.length > 0 ? 'The person holds these from day one.' : 'No Roles visible to you.'"
          :options="roles.map((role) => ({ value: role.name, label: role.name }))"
          :disabled="busy"
        />
        <small v-if="inviteProblem" class="files-problem">{{ inviteProblem }}</small>
        <template #footer><ActionButton kind="secondary" label="Cancel" icon="x" :busy="busy" @press="inviting = false" /><ActionButton kind="primary" label="Invite" icon="plus" :busy="busy" @press="invite" /></template>
    </ResponsiveDialog>

    <ResponsiveDialog v-model:open="challengeOpen" :title="`${challengeTitle}`" description="Hand this code to the user through a channel you trust. It is shown once and never again.">
        <code class="access-challenge">{{ challengeValue }}</code>
        <template #footer><ActionButton kind="secondary" label="Copy code" icon="copy" @press="copyChallenge" /><ActionButton kind="primary" label="Done" icon="check" @press="challengeOpen = false" /></template>
    </ResponsiveDialog>

    <ResponsiveDialog v-model:open="resetting" :title="`Reset ${selectedUser}`" description="A reset ends their sessions. Prove it is you with your own password first." :dirty="resetDirty">
        <div class="settings-field"><Label for="reset-password">Your password</Label><Input id="reset-password" v-model="actorPassword" type="password" autocomplete="current-password" :disabled="busy" /></div>
        <small v-if="resetProblem" class="files-problem">{{ resetProblem }}</small>
        <template #footer><ActionButton kind="secondary" label="Cancel" icon="x" :busy="busy" @press="resetting = false" /><ActionButton kind="primary" label="Start reset" icon="key-round" :busy="busy" @press="startReset" /></template>
    </ResponsiveDialog>

    <ConfirmDialog
      v-model:open="suspending"
      :title="`Suspend ${selectedUser}?`"
      body="Their sessions end at once. Reactivating later needs a review of their privileged assignments."
      confirm-label="Suspend user"
      cancel-label="Keep them"
      confirm-icon="pause"
      :danger="false"
      @confirm="suspend"
    />

    <ResponsiveDialog v-model:open="assignmentOpen" :title="assignmentForm.id ? 'Edit assignment' : 'Assign a Role to ' + selectedUser" description="One Role with its own limits and dates. Nothing applies before the preview is confirmed." :dirty="assignmentDirty">
        <SelectField
          v-if="roles.length > 0"
          id="assignment-role"
          label="Role"
          placeholder="Role"
          :options="roles.map((role) => ({ value: role.name, label: role.name }))"
          v-model="assignmentForm.role"
          :disabled="busy"
        />
        <div v-else class="settings-field"><Label for="assignment-role">Role</Label>
          <Input id="assignment-role" v-model="assignmentForm.role" autocomplete="off" spellcheck="false" :disabled="busy" />
        </div>
        <fieldset class="access-section">
          <legend>Scope limits <HelpHint term="scope" /></legend>
          <p v-if="assignmentForm.rows.length === 0" class="editor-footnote">No limits yet — this assignment covers every row. Add one to narrow it to certain values.</p>
          <div v-for="(row, index) in assignmentForm.rows" :key="index" class="access-scope-row">
            <div class="settings-field">
              <Label :for="`scope-dimension-${index}`">Field</Label>
              <Input :id="`scope-dimension-${index}`" v-model="row.dimension" list="scope-dimensions" autocomplete="off" spellcheck="false" placeholder="company" :disabled="busy" />
            </div>
            <div class="settings-field">
              <Label :for="`scope-values-${index}`">Values (comma-separated)</Label>
              <Input :id="`scope-values-${index}`" v-model="row.values" autocomplete="off" spellcheck="false" placeholder="acme, globex" :disabled="busy" />
            </div>
            <ActionButton kind="quiet" density="row" icon-only :label="`Remove limit ${index + 1}`" icon="x" :busy="busy" @press="removeScopeRow(index)" />
          </div>
          <small v-for="(row, index) in assignmentForm.rows" :key="`problem-${index}`">
            <span v-if="rowProblem(row)" class="files-problem">{{ rowProblem(row) }}</span>
          </small>
          <datalist id="scope-dimensions"><option v-for="name in scopeDimensions" :key="name" :value="name" /></datalist>
          <p v-if="scopeDimensions.length === 0" class="editor-footnote">No fields exist yet — create a Record Type first. A limit only ever matches a field of the same name.</p>
          <div class="toolbar-actions"><ActionButton kind="secondary" density="row" label="Add limit" icon="plus" :busy="busy" @press="addScopeRow" /></div>
        </fieldset>
        <fieldset class="access-section">
          <legend>Active period</legend>
          <div class="settings-field"><Label for="assignment-from">Active from</Label><Input id="assignment-from" v-model="assignmentForm.activeFrom" type="date" :disabled="busy" /></div>
          <div class="complete-toggle" role="group" aria-label="Expiry">
            <Button type="button" :variant="assignmentForm.expiresMode === 'never' ? 'secondary' : 'ghost'" size="sm" :disabled="busy" @click="assignmentForm.expiresMode = 'never'">Never expires</Button>
            <Button type="button" :variant="assignmentForm.expiresMode === 'date' ? 'secondary' : 'ghost'" size="sm" :disabled="busy" @click="assignmentForm.expiresMode = 'date'">Expires on…</Button>
          </div>
          <div v-if="assignmentForm.expiresMode === 'date'" class="settings-field"><Label for="assignment-to">Expiry date</Label><Input id="assignment-to" v-model="assignmentForm.expiresAt" type="date" :disabled="busy" /></div>
        </fieldset>
        <small v-if="assignmentProblem" class="files-problem">{{ assignmentProblem }}</small>
        <template #footer><ActionButton kind="secondary" label="Cancel" icon="x" :busy="busy" @press="assignmentOpen = false" /><ActionButton kind="primary" label="Preview" icon="eye" :busy="busy" @press="openPreview" /></template>
    </ResponsiveDialog>

    <ResponsiveDialog v-model:open="previewOpen" title="Confirm the change" :description="`Affects ${selectedUser}.`">
        <ul class="access-diff">
          <li v-for="line in previewLines" :key="line">{{ line }}</li>
          <li v-if="previewLines.length === 0">No changes.</li>
        </ul>
        <ul v-if="previewGrants.length > 0" class="access-diff">
          <li v-for="line in previewGrants" :key="line">{{ line }}</li>
        </ul>
        <template #footer><ActionButton kind="secondary" label="Back" icon="arrow-left" :busy="busy" @press="previewOpen = false" /><ActionButton kind="primary" label="Apply" icon="check" :busy="busy" :disabled-reason="previewLines.length === 0 && !busy ? 'No changes to apply.' : null" @press="confirmSaveAssignment" /></template>
    </ResponsiveDialog>

    <ResponsiveDialog v-model:open="holdersOpen" :title="`Holders of ${holdersTitle.action} on ${holdersTitle.typeKey === '*' ? 'everything' : holdersTitle.typeKey}`" :description="`Users holding ${holdersTitle.role} with a covering Grant. Usable covers dates, state and required Scope; row conditions and Scope matching still apply per Record.`">
        <p v-if="holders.length === 0" class="files-empty">No holders.</p>
        <ul v-else class="access-holders">
          <li v-for="holder in holders" :key="holder.username + holder.activeFrom">
            <strong>{{ holder.username }}</strong>
            <Badge :variant="holder.usable ? 'default' : 'outline'">{{ holder.usable ? 'Usable' : holder.state }}</Badge>
            <span class="muted-label">{{ holderScope(holder) }}</span>
            <span v-if="holder.activeFrom || holder.expiresAt" class="muted-label">{{ holder.activeFrom || '…' }} → {{ holder.expiresAt || '…' }}</span>
            <ActionButton v-if="canExplore" kind="quiet" density="row" label="Explain" icon="circle-help" @press="explainHolder(holder)" />
          </li>
        </ul>
        <template #footer><ActionButton kind="primary" label="Done" icon="check" @press="holdersOpen = false" /></template>
    </ResponsiveDialog>

    <ResponsiveDialog v-model:open="roleDialogOpen" :title="roleEditing ? 'Edit ' + roleForm.name : 'New Role'" :description="'Names, descriptions and system permissions live here. Grants are saved from the command line' + (roleEditing ? '; editing keeps them untouched' : '') + '.'" :dirty="roleDirty">
        <div class="settings-field"><Label for="role-name">Name</Label><Input id="role-name" v-model="roleForm.name" autocomplete="off" spellcheck="false" :disabled="busy || roleEditing" /></div>
        <div class="settings-field"><Label for="role-description">Description</Label><Input id="role-description" v-model="roleForm.description" autocomplete="off" :disabled="busy" /></div>
        <fieldset class="access-section">
          <legend>System permissions</legend>
          <div class="access-permissions">
            <label v-for="permission in systemPermissionOptions" :key="permission" class="access-permission">
              <Checkbox :model-value="roleForm.permissions.includes(permission)" :aria-label="permission" :disabled="busy" @update:model-value="toggleRolePermission(permission, $event)" />
              <span>{{ permission }}</span>
            </label>
          </div>
        </fieldset>
        <small v-if="roleProblem" class="files-problem">{{ roleProblem }}</small>
        <template #footer><ActionButton kind="secondary" label="Cancel" icon="x" :busy="busy" @press="roleDialogOpen = false" /><ActionButton kind="primary" :label="roleEditing ? 'Save' : 'Create'" :icon="roleEditing ? 'save' : 'plus'" :busy="busy" @press="saveRole" /></template>
    </ResponsiveDialog>

    <ConfirmDialog
      :open="deletingRole !== null"
      :title="`Delete ${deletingRole?.name}?`"
      body="Its users lose its Grants at once. The last owner cannot be stranded this way."
      confirm-label="Delete"
      cancel-label="Keep it"
      @confirm="deleteRole"
      @update:open="(value: boolean) => { if (!value) deletingRole = null }"
    />

    <ConfirmDialog
      :open="revoking !== null"
      :title="`Revoke ${revoking?.role}?`"
      :body="`${selectedUser} loses that Role at once.`"
      confirm-label="Revoke"
      cancel-label="Keep it"
      confirm-icon="undo-2"
      :danger="false"
      @confirm="revoke"
      @update:open="(value: boolean) => { if (!value) revoking = null }"
    />

    <ConfirmDialog
      :open="endingSession !== null"
      title="End this session?"
      :body="endingSession?.current ? 'This is the session you are using now. You will be signed out immediately.' : `${loadedSessionUser || 'The user'} is signed out on that device immediately.`"
      confirm-label="End session"
      cancel-label="Keep it"
      confirm-icon="log-out"
      :danger="false"
      :busy="busy"
      @confirm="confirmEndSession"
      @update:open="(value: boolean) => { if (!value) endingSession = null }"
    />
  </section>
</template>
