<script setup lang="ts">
import { computed, ref } from 'vue'
import { authClient } from '../api'
import { classify, describeFailure, type Failure } from '../apiErrors'
import type { SessionInfo } from '../gen/athena/v1/auth_pb'
import { challengeProblems, type ChallengeProblems, signInProblems, type SignInProblems } from '../signInRules'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'vue-sonner'

// The card shows the state the desk is in. A state that lasts until something
// changes (no users yet, server down, database down, a locked account) stays
// in the card; the result of an action (wrong password, copied) is a toast.
// hasUsers is false before the first user exists: the instance owner is
// created on the command line, so the page explains how instead of a form.
// pageState is what the desk found when it started: the server or the database
// is not reachable. note explains why the person is back here.
const props = defineProps<{ hasUsers: boolean; workspaceName: string; pageState: Failure | null; note: string; checking: boolean }>()
const emit = defineEmits<{ signedIn: [session: SessionInfo]; recheck: [] }>()
const ownerCommand = 'athena user invite <username> --owner'
const username = ref('')
const password = ref('')
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
    const res = await authClient.login({ username: username.value.trim(), password: password.value })
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
    <Card class="sign-in-card">
      <CardHeader>
        <CardTitle class="sign-in-title">Athena</CardTitle>
        <CardDescription>{{ workspaceName ? `Sign in to ${workspaceName}.` : 'Sign in to continue.' }}</CardDescription>
      </CardHeader>
      <CardContent>
        <div v-if="pageState" class="sign-in-state is-error" role="alert">
          <strong>{{ pageState.kind === 'database' ? 'Database not connected' : 'Server unreachable' }}</strong>
          <p>{{ pageState.message }}</p>
          <Button variant="outline" :disabled="checking" @click="emit('recheck')">{{ checking ? 'Checking…' : 'Check again' }}</Button>
        </div>
        <div v-else-if="!hasUsers" class="sign-in-setup">
          <strong>No users yet</strong>
          <p>Create the instance owner on the server, then choose a password with the shown challenge on the First login screen:</p>
          <code>{{ ownerCommand }}</code>
          <div class="toolbar-actions">
            <Button variant="outline" @click="copyCommand">Copy command</Button>
            <Button :disabled="checking" @click="emit('recheck')">{{ checking ? 'Checking…' : 'Check again' }}</Button>
          </div>
        </div>
        <form v-if="showForm && screen === 'signin'" class="sign-in-form" novalidate @submit.prevent="signIn">
          <p v-if="note" class="sign-in-state" role="status">{{ note }}</p>
          <div v-if="formState" class="sign-in-state is-error" role="alert">
            <strong>{{ formState.kind === 'locked' ? 'Account locked' : formState.kind === 'disabled' ? 'Account disabled' : formState.kind === 'database' ? 'Database not connected' : 'Server unreachable' }}</strong>
            <p>{{ formState.message }}</p>
          </div>
          <div class="settings-field">
            <Label for="sign-in-username">Username</Label>
            <Input id="sign-in-username" v-model="username" autocomplete="username" autofocus :disabled="busy" :aria-invalid="problems.username !== undefined" aria-describedby="sign-in-username-problem" @input="problems.username = undefined" />
            <small v-if="problems.username" id="sign-in-username-problem" class="files-problem">{{ problems.username }}</small>
          </div>
          <div class="settings-field">
            <Label for="sign-in-password">Password</Label>
            <Input id="sign-in-password" v-model="password" type="password" autocomplete="current-password" :disabled="busy" :aria-invalid="problems.password !== undefined" aria-describedby="sign-in-password-problem" @input="problems.password = undefined" />
            <small v-if="problems.password" id="sign-in-password-problem" class="files-problem">{{ problems.password }}</small>
          </div>
          <Button type="submit" class="sign-in-submit" :disabled="busy">{{ busy ? 'Signing in…' : 'Sign in' }}</Button>
          <Button type="button" variant="link" :disabled="busy" @click="screen = 'complete'">Have an invite or reset code? First login</Button>
        </form>
        <form v-if="showForm && screen === 'complete'" class="sign-in-form" novalidate @submit.prevent="complete">
          <div class="complete-toggle" role="group" aria-label="Challenge purpose">
            <Button type="button" :variant="purpose === 'first_login' ? 'secondary' : 'ghost'" size="sm" :disabled="busy" @click="purpose = 'first_login'">First login</Button>
            <Button type="button" :variant="purpose === 'reset' ? 'secondary' : 'ghost'" size="sm" :disabled="busy" @click="purpose = 'reset'">Password reset</Button>
          </div>
          <div class="settings-field">
            <Label for="complete-username">Username</Label>
            <Input id="complete-username" v-model="challengeUsername" autocomplete="username" :disabled="busy" @input="challengeIssues.username = undefined" />
            <small v-if="challengeIssues.username" class="files-problem">{{ challengeIssues.username }}</small>
          </div>
          <div class="settings-field">
            <Label for="complete-challenge">{{ purpose === 'reset' ? 'Reset code' : 'Invite code' }}</Label>
            <Input id="complete-challenge" v-model="challenge" autocomplete="off" spellcheck="false" :disabled="busy" @input="challengeIssues.challenge = undefined" />
            <small v-if="challengeIssues.challenge" class="files-problem">{{ challengeIssues.challenge }}</small>
          </div>
          <div class="settings-field">
            <Label for="complete-password">New password</Label>
            <Input id="complete-password" v-model="newPassword" type="password" autocomplete="new-password" :disabled="busy" @input="challengeIssues.password = undefined" />
            <small v-if="challengeIssues.password" class="files-problem">{{ challengeIssues.password }}</small>
          </div>
          <div class="settings-field">
            <Label for="complete-confirm">Repeat the password</Label>
            <Input id="complete-confirm" v-model="confirmPassword" type="password" autocomplete="new-password" :disabled="busy" @input="challengeIssues.confirm = undefined" />
            <small v-if="challengeIssues.confirm" class="files-problem">{{ challengeIssues.confirm }}</small>
          </div>
          <Button type="submit" class="sign-in-submit" :disabled="busy">{{ busy ? 'Signing in…' : 'Choose password and sign in' }}</Button>
          <Button type="button" variant="link" :disabled="busy" @click="screen = 'signin'">Back to sign in</Button>
        </form>
      </CardContent>
    </Card>
  </main>
</template>
