<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { authClient } from '../api'
import type { AccessUser, Assignment, AssignmentExplanation, Role, RoleHolder, Session } from '../gen/athena/v1/auth_pb'
import type { RecordType } from '../types'
import { actionLegend, actionLetter, describeScope, diffAssignments, parseScope, previewRoleGrants } from '../assignmentPreview'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'

// The Users & Roles page administers access through AuthService: inviting
// users, scoped dated assignments, sessions and the Permission Explorer.
// Every action is checked again on the server; the page only hides what the
// signed-in user may not do.
const props = defineProps<{ permissions: string[]; username: string; recordTypes: RecordType[] }>()

const canUsers = computed(() => props.permissions.includes('access:users'))
const canRoles = computed(() => props.permissions.includes('access:roles'))
const canAssign = computed(() => props.permissions.includes('access:assign'))
const canExplore = computed(() => props.permissions.includes('access:explore'))

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

function when(iso: string): string {
  return iso ? new Date(iso).toLocaleString() : ''
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
    if (err instanceof ConnectError && err.code === Code.Unavailable) unavailable.value = err.rawMessage
  } finally {
    loaded.value = true
  }
}

// The selected user and their assignments.
const selectedUser = ref('')
const assignments = ref<Assignment[]>([])
const assignmentsLoaded = ref(false)

async function selectUser(username: string) {
  selectedUser.value = username
  assignmentsLoaded.value = false
  if (!username) {
    assignments.value = []
    return
  }
  try {
    assignments.value = (await authClient.listAssignments({ username })).assignments
  } catch (err) {
    toast.error(describe(err, 'load the assignments'))
  } finally {
    assignmentsLoaded.value = true
  }
}

const selected = computed(() => users.value.find((user) => user.username === selectedUser.value) ?? null)

// The workspace root account is fully visible here but never editable from
// the desk: invites, resets, suspension and assignments cannot touch it.
const rootUsername = 'administrator'
const isRootSelected = computed(() => selected.value?.username === rootUsername)

// Field names across Record Types feed the Scope dimension suggestions: a
// dimension only ever matches a field of the same name.
const scopeDimensions = computed(() => {
  const names = new Set<string>()
  for (const recordType of props.recordTypes) for (const field of recordType.fields) names.add(field.name)
  return [...names].sort()
})

// Inviting. The Challenge is shown once in its own dialog.
const inviting = ref(false)
const inviteForm = ref({ username: '', displayName: '', roles: '' })
const inviteProblem = ref('')

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
  const names = inviteForm.value.roles
    .split(',')
    .map((name) => name.trim())
    .filter((name) => name !== '')
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
    inviteForm.value = { username: '', displayName: '', roles: '' }
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

async function openHolders(role: string, typeKey: string, action: string) {
  busy.value = true
  try {
    holders.value = (await authClient.listRoleHolders({ role, recordTypeKey: typeKey, action })).holders
    holdersTitle.value = { role, typeKey, action }
    holdersOpen.value = true
  } catch (err) {
    toast.error(describe(err, 'load the holders'))
  } finally {
    busy.value = false
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
const sessions = ref<Session[]>([])

async function loadSessions() {
  if (!sessionUser.value.trim()) return
  busy.value = true
  try {
    sessions.value = (await authClient.listSessions({ username: sessionUser.value.trim() })).sessions
  } catch (err) {
    toast.error(describe(err, 'load the sessions'))
  } finally {
    busy.value = false
  }
}

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

// Permission Explorer. With a Record the verdicts test row conditions and
// Scopes against it; without one they cover dates, required Scope and
// Grants only.
const grantActions = ['read', 'create', 'write', 'delete', 'submit', 'cancel', 'amend', 'audit']
const exploreForm = ref({ username: props.username, action: 'read', recordType: '', recordId: '' })
const explanation = ref<{ allowed: boolean; assignments: AssignmentExplanation[] } | null>(null)

async function explain() {
  if (!exploreForm.value.username.trim() || !exploreForm.value.recordType.trim()) {
    toast.error('Enter a username and a Record Type.')
    return
  }
  busy.value = true
  try {
    explanation.value = await authClient.explainAccess({
      username: exploreForm.value.username.trim(),
      action: exploreForm.value.action,
      recordTypeKey: exploreForm.value.recordType.trim(),
      recordId: exploreForm.value.recordId.trim(),
    })
  } catch (err) {
    explanation.value = null
    toast.error(describe(err, 'explain the access'))
  } finally {
    busy.value = false
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
      <div><h1>Users &amp; Roles</h1><p class="lead">Invite users, hand out scoped Roles, inspect sessions and explain access decisions.</p></div>
      <div v-if="tab === 'users' && canUsers && !unavailable" class="toolbar-actions"><Button @click="inviting = true">＋ Invite user</Button></div>
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
        <div class="access-split">
          <div class="access-main">
            <p v-if="loaded && users.length === 0" class="files-empty">No users yet.</p>
            <Table v-else>
              <TableHeader><TableRow><TableHead>Username</TableHead><TableHead>Display name</TableHead><TableHead>State</TableHead><TableHead>Roles</TableHead><TableHead>Last sign-in</TableHead></TableRow></TableHeader>
              <TableBody>
                <TableRow v-for="user in users" :key="user.username" class="cursor-pointer" :data-state="user.username === selectedUser ? 'selected' : undefined" @click="selectUser(user.username === selectedUser ? '' : user.username)">
                  <TableCell>{{ user.username }} <Badge v-if="user.owner" variant="secondary">Owner</Badge></TableCell>
                  <TableCell>{{ user.displayName }}</TableCell>
                  <TableCell><Badge :variant="stateVariant(user.state)">{{ user.state || 'active' }}</Badge><Badge v-if="user.locked" variant="destructive">Locked</Badge></TableCell>
                  <TableCell>{{ user.roles.join(', ') }}</TableCell>
                  <TableCell>{{ when(user.lastLoginAt) }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
          <aside v-if="selected" class="access-drawer" aria-label="User details">
            <div class="record-editor-head">
              <h2 class="files-title">{{ selected.username }}</h2>
              <Badge :variant="stateVariant(selected.state)">{{ selected.state || 'active' }}</Badge>
              <div class="toolbar-actions"><Button variant="ghost" size="sm" aria-label="Close details" @click="selectUser('')">✕</Button></div>
            </div>
            <p v-if="isRootSelected" class="editor-footnote">The root account is managed outside the desk and cannot be changed here.</p>
            <div v-else class="toolbar-actions access-drawer-actions">
              <Button v-if="canUsers && selected.state === 'invited'" variant="outline" size="sm" :disabled="busy" @click="resend(selected.username)">Resend invite</Button>
              <Button v-if="canUsers && (selected.state === 'active' || selected.state === 'reset_required')" variant="outline" size="sm" :disabled="busy" @click="resetting = true">Reset</Button>
              <Button v-if="canUsers" variant="outline" size="sm" :disabled="busy || selected.disabled" @click="suspending = true">Suspend</Button>
              <Button v-if="canUsers" variant="outline" size="sm" :disabled="busy || !selected.disabled" @click="reactivate">Reactivate</Button>
              <Button v-if="canAssign" size="sm" :disabled="busy" @click="openNewAssignment">＋ Assign Role</Button>
            </div>
            <p v-if="assignmentsLoaded && assignments.length === 0" class="files-empty">No assignments. {{ canAssign && !isRootSelected ? 'Assign a Role.' : '' }}</p>
            <ul v-else class="access-assignments">
              <li v-for="assignment in assignments" :key="assignment.id" class="access-assignment">
                <Badge>{{ assignment.role }}</Badge>
                <span class="muted-label">{{ scopeText(assignment) }}</span>
                <span v-if="assignment.activeFrom || assignment.expiresAt" class="muted-label">{{ assignment.activeFrom || '…' }} → {{ assignment.expiresAt || '…' }}</span>
                <span v-if="canAssign && !isRootSelected" class="toolbar-actions">
                  <Button variant="ghost" size="sm" :disabled="busy" @click="openEditAssignment(assignment)">Edit</Button>
                  <Button variant="ghost" size="sm" :disabled="busy" @click="revoking = assignment">Revoke</Button>
                </span>
              </li>
            </ul>
          </aside>
        </div>
      </TabsContent>
      <TabsContent value="roles">
        <div class="access-split">
          <div class="access-main">
            <div class="toolbar-actions access-tab-actions"><Button size="sm" :disabled="busy" @click="openNewRole">＋ New Role</Button></div>
            <p v-if="loaded && roles.length === 0" class="files-empty">No Roles yet.</p>
            <Table v-else>
              <TableHeader><TableRow><TableHead>Role</TableHead><TableHead>Description</TableHead><TableHead>System permissions</TableHead><TableHead>Grants</TableHead></TableRow></TableHeader>
              <TableBody>
                <TableRow v-for="role in roles" :key="role.name" class="cursor-pointer" :data-state="role.name === selectedRole ? 'selected' : undefined" @click="selectedRole = role.name === selectedRole ? '' : role.name">
                  <TableCell>{{ role.name }}</TableCell>
                  <TableCell>{{ role.description }}</TableCell>
                  <TableCell>{{ role.systemPermissions.join(', ') }}</TableCell>
                  <TableCell>{{ role.grants.length }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
          <aside v-if="roleDetail" class="access-drawer" aria-label="Role details">
            <div class="record-editor-head">
              <h2 class="files-title">{{ roleDetail.name }}</h2>
              <div class="toolbar-actions">
                <Button variant="ghost" size="sm" :disabled="busy" @click="openEditRole(roleDetail)">Edit</Button>
                <Button variant="ghost" size="sm" :disabled="busy" @click="deletingRole = roleDetail">Delete</Button>
                <Button variant="ghost" size="sm" aria-label="Close details" @click="selectedRole = ''">✕</Button>
              </div>
            </div>
            <p v-if="roleDetail.description" class="muted-label">{{ roleDetail.description }}</p>
            <p v-if="roleDetail.grants.length === 0" class="files-empty">No Grants. Roles gain Grants from the command line.</p>
            <ul v-else class="access-grants">
              <li v-for="(grant, index) in roleDetail.grants" :key="index">
                <strong>{{ grant.actions.join(', ') }}</strong> {{ grant.recordTypeKey }} <span class="muted-label">level {{ grant.permissionLevel }}</span>
                <span v-if="grant.condition" class="muted-label">where {{ grant.condition }}</span>
                <Badge v-if="grant.separateDuties" variant="outline">Separate duties</Badge>
              </li>
            </ul>
          </aside>
        </div>
      </TabsContent>
      <TabsContent value="permissions">
        <p v-if="loaded && roles.length === 0" class="files-empty">No Roles yet.</p>
        <Table v-else>
          <TableHeader><TableRow><TableHead>Record Type</TableHead><TableHead v-for="role in roles" :key="role.name">{{ role.name }}</TableHead></TableRow></TableHeader>
          <TableBody>
            <TableRow v-for="typeKey in matrixTypes" :key="typeKey">
              <TableCell>{{ typeKey === '*' ? 'All record types' : typeKey }}</TableCell>
              <TableCell v-for="role in roles" :key="role.name">
                <span v-if="cellActions(role, typeKey).length === 0" class="muted-label">·</span>
                <Button v-for="action in cellActions(role, typeKey)" :key="action" variant="ghost" size="sm" class="access-letter" :disabled="busy" :title="`${action} ${typeKey === '*' ? 'everything' : typeKey} — show holders`" @click="openHolders(role.name, typeKey, action)">{{ actionLetter(action) }}</Button>
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
          <div class="toolbar-actions"><Button variant="outline" :disabled="busy" @click="loadSessions">List sessions</Button></div>
        </div>
        <p v-if="sessions.length === 0" class="files-empty">No live sessions.</p>
        <Table v-else>
          <TableHeader><TableRow><TableHead>Started</TableHead><TableHead>Last seen</TableHead><TableHead>Expires</TableHead><TableHead>Device</TableHead><TableHead></TableHead><TableHead></TableHead></TableRow></TableHeader>
          <TableBody>
            <TableRow v-for="session in sessions" :key="session.id">
              <TableCell>{{ when(session.createdAt) }}</TableCell>
              <TableCell>{{ when(session.lastSeenAt) }}</TableCell>
              <TableCell>{{ when(session.expiresAt) }}</TableCell>
              <TableCell>{{ session.device || '—' }}</TableCell>
              <TableCell><Badge v-if="session.current" variant="secondary">This device</Badge></TableCell>
              <TableCell><Button variant="ghost" size="sm" :disabled="busy" @click="revokeSession(session.id)">End</Button></TableCell>
            </TableRow>
          </TableBody>
        </Table>
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
          <div class="toolbar-actions"><Button :disabled="busy" @click="explain">Explain</Button></div>
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

    <Dialog :open="inviting" @update:open="(value: boolean) => { inviting = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>Invite user</DialogTitle><DialogDescription>The user signs in for the first time with a one-time challenge, shown once.</DialogDescription></DialogHeader>
        <div class="settings-field"><Label for="invite-username">Username</Label><Input id="invite-username" v-model="inviteForm.username" autocomplete="off" spellcheck="false" :disabled="busy" /></div>
        <div class="settings-field"><Label for="invite-display">Display name</Label><Input id="invite-display" v-model="inviteForm.displayName" autocomplete="off" :disabled="busy" /></div>
        <div class="settings-field"><Label for="invite-roles">Roles (comma-separated, optional)</Label><Input id="invite-roles" v-model="inviteForm.roles" autocomplete="off" spellcheck="false" :disabled="busy" /></div>
        <small v-if="inviteProblem" class="files-problem">{{ inviteProblem }}</small>
        <DialogFooter><Button variant="outline" :disabled="busy" @click="inviting = false">Cancel</Button><Button :disabled="busy" @click="invite">Invite</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog :open="challengeOpen" @update:open="(value: boolean) => { challengeOpen = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>{{ challengeTitle }}</DialogTitle><DialogDescription>Hand this challenge to the user through a channel you trust. It is shown once and never again.</DialogDescription></DialogHeader>
        <code class="access-challenge">{{ challengeValue }}</code>
        <DialogFooter><Button variant="outline" @click="copyChallenge">Copy challenge</Button><Button @click="challengeOpen = false">Done</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog :open="resetting" @update:open="(value: boolean) => { resetting = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>Reset {{ selectedUser }}</DialogTitle><DialogDescription>A reset ends their sessions. Prove it is you with your own password first.</DialogDescription></DialogHeader>
        <div class="settings-field"><Label for="reset-password">Your password</Label><Input id="reset-password" v-model="actorPassword" type="password" autocomplete="current-password" :disabled="busy" /></div>
        <small v-if="resetProblem" class="files-problem">{{ resetProblem }}</small>
        <DialogFooter><Button variant="outline" :disabled="busy" @click="resetting = false">Cancel</Button><Button :disabled="busy" @click="startReset">Start reset</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <AlertDialog :open="suspending" @update:open="(value: boolean) => { suspending = value }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Suspend {{ selectedUser }}?</AlertDialogTitle><AlertDialogDescription>Their sessions end at once. Reactivating later needs a review of their privileged assignments.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep them</AlertDialogCancel><AlertDialogAction variant="destructive" @click="suspend">Suspend user</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <Dialog :open="assignmentOpen" @update:open="(value: boolean) => { assignmentOpen = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>{{ assignmentForm.id ? 'Edit assignment' : `Assign a Role to ${selectedUser}` }}</DialogTitle><DialogDescription>One Role with its own limits and dates. Nothing applies before the preview is confirmed.</DialogDescription></DialogHeader>
        <div class="settings-field"><Label for="assignment-role">Role</Label>
          <Select v-if="roles.length > 0" v-model="assignmentForm.role" :disabled="busy">
            <SelectTrigger id="assignment-role"><SelectValue placeholder="Role" /></SelectTrigger>
            <SelectContent><SelectItem v-for="role in roles" :key="role.name" :value="role.name">{{ role.name }}</SelectItem></SelectContent>
          </Select>
          <Input v-else id="assignment-role" v-model="assignmentForm.role" autocomplete="off" spellcheck="false" :disabled="busy" />
        </div>
        <fieldset class="access-section">
          <legend>Scope limits</legend>
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
            <Button variant="ghost" size="sm" :disabled="busy" :aria-label="`Remove limit ${index + 1}`" @click="removeScopeRow(index)">✕</Button>
          </div>
          <small v-for="(row, index) in assignmentForm.rows" :key="`problem-${index}`">
            <span v-if="rowProblem(row)" class="files-problem">{{ rowProblem(row) }}</span>
          </small>
          <datalist id="scope-dimensions"><option v-for="name in scopeDimensions" :key="name" :value="name" /></datalist>
          <p v-if="scopeDimensions.length === 0" class="editor-footnote">No fields exist yet — create a Record Type first. A limit only ever matches a field of the same name.</p>
          <div class="toolbar-actions"><Button variant="outline" size="sm" :disabled="busy" @click="addScopeRow">＋ Add limit</Button></div>
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
        <DialogFooter><Button variant="outline" :disabled="busy" @click="assignmentOpen = false">Cancel</Button><Button :disabled="busy" @click="openPreview">Preview</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog :open="previewOpen" @update:open="(value: boolean) => { previewOpen = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>Confirm the change</DialogTitle><DialogDescription>Affects {{ selectedUser }}.</DialogDescription></DialogHeader>
        <ul class="access-diff">
          <li v-for="line in previewLines" :key="line">{{ line }}</li>
          <li v-if="previewLines.length === 0">No changes.</li>
        </ul>
        <ul v-if="previewGrants.length > 0" class="access-diff">
          <li v-for="line in previewGrants" :key="line">{{ line }}</li>
        </ul>
        <DialogFooter><Button variant="outline" :disabled="busy" @click="previewOpen = false">Back</Button><Button :disabled="busy || previewLines.length === 0" @click="confirmSaveAssignment">Apply</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog :open="holdersOpen" @update:open="(value: boolean) => { holdersOpen = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>Holders of {{ holdersTitle.action }} on {{ holdersTitle.typeKey === '*' ? 'everything' : holdersTitle.typeKey }}</DialogTitle><DialogDescription>Users holding {{ holdersTitle.role }} with a covering Grant. Usable covers dates, state and required Scope; row conditions and Scope matching still apply per Record.</DialogDescription></DialogHeader>
        <p v-if="holders.length === 0" class="files-empty">No holders.</p>
        <ul v-else class="access-holders">
          <li v-for="holder in holders" :key="holder.username + holder.activeFrom">
            <strong>{{ holder.username }}</strong>
            <Badge :variant="holder.usable ? 'default' : 'outline'">{{ holder.usable ? 'Usable' : holder.state }}</Badge>
            <span class="muted-label">{{ holderScope(holder) }}</span>
            <span v-if="holder.activeFrom || holder.expiresAt" class="muted-label">{{ holder.activeFrom || '…' }} → {{ holder.expiresAt || '…' }}</span>
            <Button v-if="canExplore" variant="ghost" size="sm" @click="explainHolder(holder)">Explain</Button>
          </li>
        </ul>
        <DialogFooter><Button @click="holdersOpen = false">Done</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog :open="roleDialogOpen" @update:open="(value: boolean) => { roleDialogOpen = value }">
      <DialogContent>
        <DialogHeader><DialogTitle>{{ roleEditing ? `Edit ${roleForm.name}` : 'New Role' }}</DialogTitle><DialogDescription>Names, descriptions and system permissions live here. Grants are saved from the command line{{ roleEditing ? '; editing keeps them untouched' : '' }}.</DialogDescription></DialogHeader>
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
        <DialogFooter><Button variant="outline" :disabled="busy" @click="roleDialogOpen = false">Cancel</Button><Button :disabled="busy" @click="saveRole">{{ roleEditing ? 'Save' : 'Create' }}</Button></DialogFooter>
      </DialogContent>
    </Dialog>

    <AlertDialog :open="deletingRole !== null" @update:open="(value: boolean) => { if (!value) deletingRole = null }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Delete {{ deletingRole?.name }}?</AlertDialogTitle><AlertDialogDescription>Its users lose its Grants at once. The last owner cannot be stranded this way.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep it</AlertDialogCancel><AlertDialogAction variant="destructive" @click="deleteRole">Delete</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <AlertDialog :open="revoking !== null" @update:open="(value: boolean) => { if (!value) revoking = null }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Revoke {{ revoking?.role }}?</AlertDialogTitle><AlertDialogDescription>{{ selectedUser }} loses that Role at once.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep it</AlertDialogCancel><AlertDialogAction variant="destructive" @click="revoke">Revoke</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </section>
</template>
