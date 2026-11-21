<script setup lang="ts">
import { computed, ref } from 'vue'
import { authClient } from '../api'
import { classify, describeFailure, type Failure } from '../apiErrors'
import type { SessionInfo } from '../gen/athena/v1/auth_pb'
import { challengeProblems, type ChallengeProblems, signInProblems, type SignInProblems } from '../signInRules'
import ArtFigure from '@/art/ArtFigure.vue'
import AsciiMark from '@/art/AsciiMark.vue'
import ActionButton from '@/components/ActionButton.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'
import CheckboxField from '@/components/fields/CheckboxField.vue'
import TextField from '@/components/fields/TextField.vue'
import WorkspaceMark from '@/components/WorkspaceMark.vue'
import { identity } from '@/identity'
import { toast } from 'vue-sonner'

// The card shows the state the desk is in. A state that lasts until something
// changes (no users yet, server down, database down, a locked account) stays
// in the card; the result of an action (wrong password, copied) is a toast.
// hasUsers is false before the first user exists: the instance owner is
// created on the command line, so the page explains how instead of a form.
// pageState is what the desk found when it started: the server or the database
// is not reachable. note explains why the person is back here.
const props = defineProps<{ hasUsers: boolean; pageState: Failure | null; note: string; checking: boolean }>()
const emit = defineEmits<{ signedIn: [session: SessionInfo]; recheck: [] }>()
const ownerCommand = 'athena user invite <username> --owner'
const username = ref('')
const password = ref('')
const rememberDevice = ref(false)
const busy = ref(false)
const problems = ref<SignInProblems>({})
const formState = ref<Failure | null>(null)

// First login and password reset share one completion screen: the user proves
// the one-time challenge and chooses their own password, then signs straight
// in. Purpose selects which Challenge completes.
const screen = ref<'signin' | 'complete'>('signin')
const purpose = ref<'first_login' | 'reset'>('first_login')
const challengeUsername = ref('')
const challenge = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const challengeIssues = ref<ChallengeProblems>({})

const showForm = computed(() => props.hasUsers && props.pageState === null)

async function copyCommand() {
  try {
    await navigator.clipboard.writeText(ownerCommand)
    toast.success('Command copied.')
  } catch {
    toast.error('Could not copy. Select the command and copy it.')
  }
}

function validate() {
  problems.value = signInProblems(username.value, password.value)
  return Object.keys(problems.value).length === 0
}

async function signIn() {
  formState.value = null
  if (!validate()) return
  busy.value = true
  try {
    const res = await authClient.login({ username: username.value.trim(), password: password.value, rememberDevice: rememberDevice.value })
    password.value = ''
    if (res.session) emit('signedIn', res.session)
  } catch (err) {
    const failure = classify(err, { signIn: true, username: username.value.trim().toLowerCase() })
    password.value = ''
    if (failure.persistent) formState.value = failure
    else if (failure.kind === 'wrong-credentials') toast.error(failure.message)
    else toast.error(failure.kind === 'unexpected' ? describeFailure(failure) : failure.message)
  } finally {
    busy.value = false
  }
}

function validateChallenge() {
  challengeIssues.value = challengeProblems(challengeUsername.value, challenge.value, newPassword.value, confirmPassword.value)
  return Object.keys(challengeIssues.value).length === 0
}

async function complete() {
  if (!validateChallenge()) return
  busy.value = true
  try {
    const res = await authClient.completeChallenge({
      username: challengeUsername.value.trim(),
      purpose: purpose.value,
      challenge: challenge.value.trim(),
      newPassword: newPassword.value,
    })
    newPassword.value = ''
    confirmPassword.value = ''
    if (res.session) emit('signedIn', res.session)
  } catch (err) {
    const failure = classify(err, { signIn: true, username: challengeUsername.value.trim().toLowerCase() })
    newPassword.value = ''
    confirmPassword.value = ''
    if (failure.persistent) formState.value = failure
    else toast.error(failure.kind === 'unexpected' ? describeFailure(failure) : failure.message)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <main class="sign-in">
    <div class="sign-in-inner">
      <ArtFigure art-key="signin" />
      <Card class="sign-in-card">
        <CardHeader class="items-center justify-items-center text-center">
          <WorkspaceMark :height="48" />
          <h1>{{ identity.displayName || 'Athena' }}</h1>
          <CardDescription class="text-base">Sign in to continue.</CardDescription>
        </CardHeader>
        <CardContent>
          <div v-if="pageState" class="sign-in-state is-error" role="alert">
            <strong>{{ pageState.kind === 'database' ? 'Database not connected' : 'Server unreachable' }}</strong>
            <p>{{ pageState.message }}</p>
            <ActionButton kind="secondary" :label="checking ? 'Checking…' : 'Check again'" icon="refresh-ccw" :busy="checking" @press="emit('recheck')" />
          </div>
          <div v-else-if="!hasUsers" class="sign-in-setup">
            <strong>No users yet</strong>
            <p>Create the owner on the server, then choose a password with the shown code on the First login screen:</p>
            <code>{{ ownerCommand }}</code>
            <div class="toolbar-actions">
              <ActionButton kind="secondary" label="Copy command" icon="copy" @press="copyCommand" />
              <ActionButton kind="primary" :label="checking ? 'Checking…' : 'Check again'" icon="refresh-ccw" :busy="checking" @press="emit('recheck')" />
            </div>
          </div>
          <form v-if="showForm && screen === 'signin'" class="sign-in-form" novalidate @submit.prevent="signIn">
            <p v-if="note" class="sign-in-state" role="status">{{ note }}</p>
            <div v-if="formState" class="sign-in-state is-error" role="alert">
              <strong>{{ formState.kind === 'locked' ? 'Account locked' : formState.kind === 'disabled' ? 'Account disabled' : formState.kind === 'database' ? 'Database not connected' : 'Server unreachable' }}</strong>
              <p>{{ formState.message }}</p>
            </div>
            <TextField id="sign-in-username" label="Username" v-model="username" autocomplete="username" autofocus :spellcheck="false" :error="problems.username" :disabled="busy" @update:model-value="problems.username = undefined" />
            <TextField id="sign-in-password" label="Password" type="password" v-model="password" autocomplete="current-password" :error="problems.password" :disabled="busy" @update:model-value="problems.password = undefined" />
            <CheckboxField id="sign-in-remember" label="Keep me signed in on this device" v-model="rememberDevice" help="Stay signed in for 7 days. Builders and root always use shorter sessions." :disabled="busy" />
            <ActionButton type="submit" kind="primary" class="sign-in-submit" :label="busy ? 'Signing in…' : 'Sign in'" icon="key-round" :busy="busy" />
            <button type="button" class="sign-in-link" :disabled="busy" @click="screen = 'complete'">Have an invite or reset code? First login</button>
          </form>
          <form v-if="showForm && screen === 'complete'" class="sign-in-form" novalidate @submit.prevent="complete">
            <div class="complete-toggle" role="group" aria-label="Code purpose">
              <Button type="button" :variant="purpose === 'first_login' ? 'secondary' : 'ghost'" size="sm" :disabled="busy" @click="purpose = 'first_login'">First login</Button>
              <Button type="button" :variant="purpose === 'reset' ? 'secondary' : 'ghost'" size="sm" :disabled="busy" @click="purpose = 'reset'">Password reset</Button>
            </div>
            <TextField id="complete-username" label="Username" v-model="challengeUsername" autocomplete="username" :error="challengeIssues.username" :disabled="busy" @update:model-value="challengeIssues.username = undefined" />
            <TextField id="complete-challenge" :label="purpose === 'reset' ? 'Reset code' : 'Invite code'" v-model="challenge" autocomplete="off" :error="challengeIssues.challenge" :disabled="busy" @update:model-value="challengeIssues.challenge = undefined" />
            <TextField id="complete-password" label="New password" type="password" v-model="newPassword" autocomplete="new-password" :error="challengeIssues.password" :disabled="busy" @update:model-value="challengeIssues.password = undefined" />
            <TextField id="complete-confirm" label="Repeat the password" type="password" v-model="confirmPassword" autocomplete="new-password" :error="challengeIssues.confirm" :disabled="busy" @update:model-value="challengeIssues.confirm = undefined" />
            <ActionButton type="submit" kind="primary" class="sign-in-submit" :label="busy ? 'Signing in…' : 'Choose password and sign in'" icon="check" :busy="busy" />
            <button type="button" class="sign-in-link" :disabled="busy" @click="screen = 'signin'">Back to sign in</button>
          </form>
        </CardContent>
      </Card>
      <footer class="sign-in-footer">
        <AsciiMark art-key="signin-mark" />
      </footer>
    </div>
  </main>
</template>
