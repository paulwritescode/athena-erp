// Setup wizard step 5: owner (B11 slice 2). Invites the first owner with
// the Instance Owner role by calling InviteUser directly. The invite code
// shows once, inline, with a copy action. Optional: skipping still
// finishes the wizard, and readiness shows the owner skipped.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { authClient } from '@/api'
import { classify, describeFailure } from '@/apiErrors'
import HelpHint from '@/copy/HelpHint.vue'
import { isRootUsername } from '@/rootUser'
import ActionButton from '@/components/ActionButton.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import TextField from '@/components/fields/TextField.vue'
import SetupFooter from './SetupFooter.vue'
import { checkDisplayName, checkUsername, normalizeUsername } from './setupModel'

const props = defineProps<{ canAccess: boolean }>()
const emit = defineEmits<{
  back: []
  done: [result: { owners: string[]; skipped: boolean }]
}>()

const loaded = ref(false)
const loadError = ref('')
const inviting = ref(false)
const owners = ref<string[]>([])
const invitingAnother = ref(false)
const username = ref('')
const displayName = ref('')
const usernameProblem = ref('')
const displayNameProblem = ref('')
const inviteProblem = ref('')
const invitedUser = ref('')
const challenge = ref('')

const showForm = computed(() => owners.value.length === 0 || invitingAnother.value)

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const res = await authClient.listUsers({})
    owners.value = res.users.filter((user) => user.owner && !isRootUsername(user.username)).map((user) => user.username)
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the owners.'
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  if (props.canAccess) void refresh()
  else loaded.value = true
})

async function copyChallenge() {
  try {
    await navigator.clipboard.writeText(challenge.value)
    toast.success('Invite code copied.')
  } catch {
    toast.error('Could not copy. Select the code and copy it.')
  }
}

async function submit() {
  if (challenge.value || (!showForm.value && owners.value.length > 0)) {
    emit('done', { owners: owners.value, skipped: false })
    return
  }
  inviteProblem.value = ''
  usernameProblem.value = checkUsername(username.value) ?? ''
  displayNameProblem.value = checkDisplayName(displayName.value.trim()) ?? ''
  if (usernameProblem.value || displayNameProblem.value) return
  inviting.value = true
  try {
    // Step-up for the role grant is handled by the shared api
    // interceptor: it asks for the password and retries this call.
    const res = await authClient.inviteUser({
      username: normalizeUsername(username.value),
      displayName: displayName.value.trim(),
      roles: ['Instance Owner'],
    })
    invitedUser.value = res.user?.username ?? normalizeUsername(username.value)
    if (!owners.value.includes(invitedUser.value)) owners.value = [...owners.value, invitedUser.value]
    challenge.value = res.challenge
    toast.success(`Invited ${invitedUser.value}.`)
  } catch (err) {
    inviteProblem.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not invite the owner.'
  } finally {
    inviting.value = false
  }
}
</script>

<template>
  <div>
    <h2>Invite the first owner <HelpHint term="owner" /></h2>
    <p class="text-muted-foreground mt-1 text-sm">Owners invite others and approve the biggest changes. The invite code shows once — or skip it for now.</p>
    <LoadingSkeleton v-if="!loaded" class="mt-6" />
    <div v-else-if="!canAccess" class="mt-6">
      <p class="text-sm">You need the access permission to invite people. Ask your administrator.</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', { owners: [], skipped: true })" />
    </div>
    <div v-else-if="loadError" class="mt-6">
      <p class="text-sm" role="alert">{{ loadError }}</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', { owners: [], skipped: false })">
        <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
      </SetupFooter>
    </div>
    <div v-else class="mt-6 space-y-4">
      <p v-if="owners.length > 0" class="text-sm">Owners so far: {{ owners.join(', ') }}.</p>
      <div v-if="challenge" class="space-y-2 rounded-[var(--radius)] border p-4">
        <p class="text-sm font-medium">Invite code for {{ invitedUser }} — it shows once.</p>
        <div class="flex flex-wrap items-center gap-2">
          <code class="bg-muted rounded-[var(--radius)] px-2 py-1 font-mono text-sm">{{ challenge }}</code>
          <ActionButton kind="secondary" density="row" label="Copy" icon="copy" @press="copyChallenge" />
        </div>
        <p class="muted-label">Share it with {{ invitedUser }}. They choose their own password with it at first sign-in.</p>
      </div>
      <template v-if="showForm && !challenge">
        <TextField
          id="setup-owner-username"
          label="Username"
          help="Lowercase, no spaces."
          placeholder="alice"
          autocomplete="off"
          :error="usernameProblem || undefined"
          :model-value="username"
          @update:model-value="username = $event.toLowerCase(); usernameProblem = ''"
        />
        <TextField
          id="setup-owner-name"
          label="Display name"
          :optional="true"
          help="The name people see."
          placeholder="Alice"
          autocomplete="off"
          :error="displayNameProblem || undefined"
          :model-value="displayName"
          @update:model-value="displayName = $event; displayNameProblem = ''"
        />
        <p class="muted-label">Invited as Owner.</p>
        <div v-if="owners.length > 0">
          <ActionButton kind="quiet" label="Back to owners" icon="undo-2" @press="invitingAnother = false" />
        </div>
      </template>
      <div v-else-if="!challenge && owners.length > 0">
        <ActionButton kind="quiet" label="Invite another owner" icon="plus" @press="invitingAnother = true" />
      </div>
      <p v-if="inviteProblem" class="text-sm text-destructive" role="alert">{{ inviteProblem }}</p>
      <SetupFooter
        :primary-label="challenge || (!showForm && owners.length > 0) ? 'Continue' : 'Invite owner & continue'"
        :primary-icon="challenge || (!showForm && owners.length > 0) ? 'chevron-right' : 'plus'"
        :primary-busy="inviting"
        @back="emit('back')"
        @primary="submit"
      >
        <ActionButton
          v-if="!challenge && owners.length === 0"
          kind="quiet"
          label="Skip for now"
          icon="chevron-right"
          @press="emit('done', { owners: [], skipped: true })"
        />
      </SetupFooter>
    </div>
  </div>
</template>
