<script setup lang="ts">
import { ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { authClient } from '../api'
import type { SessionInfo } from '../gen/athena/v1/auth_pb'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'vue-sonner'

// hasUsers is false before the first user exists: the instance owner is
// created on the command line, so the page explains how instead of a form.
defineProps<{ hasUsers: boolean; workspaceName: string }>()
const emit = defineEmits<{ signedIn: [session: SessionInfo] }>()

const username = ref('')
const password = ref('')
const busy = ref(false)

async function signIn() {
  if (!username.value.trim() || !password.value) {
    toast.error('Enter your username and password.')
    return
  }
  busy.value = true
  try {
    const res = await authClient.login({ username: username.value, password: password.value })
    password.value = ''
    if (res.session) emit('signedIn', res.session)
  } catch (err) {
    // The server gives one message for every sign-in failure, so it does not
    // reveal whether the user exists or is locked.
    toast.error(err instanceof ConnectError && err.code === Code.Unauthenticated ? err.rawMessage : 'Could not sign in. Try again.')
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
        <div v-if="!hasUsers" class="sign-in-setup">
          <p>No users exist yet. Create the instance owner on the server, then reload this page:</p>
          <code>athena user create &lt;username&gt; --owner</code>
        </div>
        <form v-else class="sign-in-form" @submit.prevent="signIn">
          <div class="settings-field"><Label for="sign-in-username">Username</Label><Input id="sign-in-username" v-model="username" autocomplete="username" autofocus /></div>
          <div class="settings-field"><Label for="sign-in-password">Password</Label><Input id="sign-in-password" v-model="password" type="password" autocomplete="current-password" /></div>
          <Button type="submit" class="sign-in-submit" :disabled="busy">{{ busy ? 'Signing in…' : 'Sign in' }}</Button>
        </form>
      </CardContent>
    </Card>
  </main>
</template>
