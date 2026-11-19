<script setup lang="ts">
import { ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { authClient } from '../api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'vue-sonner'

// The server applies the password policy (12 to 256 characters, not the
// username) and ends the user's other sessions.
const open = defineModel<boolean>('open', { required: true })
const current = ref('')
const next = ref('')
const repeat = ref('')
const busy = ref(false)

function reset() {
  current.value = ''
  next.value = ''
  repeat.value = ''
}

async function change() {
  if (next.value !== repeat.value) {
    toast.error('The new passwords do not match.')
    return
  }
  busy.value = true
  try {
    await authClient.changePassword({ currentPassword: current.value, newPassword: next.value })
    toast.success('Password changed. Your other sessions were signed out.')
    reset()
    open.value = false
  } catch (err) {
    toast.error(err instanceof ConnectError ? `Could not change the password: ${err.rawMessage}` : 'Could not change the password.')
  } finally {
    busy.value = false
  }
}

function onOpenChange(value: boolean) {
  if (!value) reset()
  open.value = value
}
</script>

<template>
  <Dialog :open="open" @update:open="onOpenChange">
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Change password</DialogTitle>
        <DialogDescription>Use at least 12 characters. Your other signed-in sessions end.</DialogDescription>
      </DialogHeader>
      <form id="change-password" class="sign-in-form" @submit.prevent="change">
        <div class="settings-field"><Label for="cp-current">Current password</Label><Input id="cp-current" v-model="current" type="password" autocomplete="current-password" /></div>
        <div class="settings-field"><Label for="cp-new">New password</Label><Input id="cp-new" v-model="next" type="password" autocomplete="new-password" /></div>
        <div class="settings-field"><Label for="cp-repeat">Repeat the new password</Label><Input id="cp-repeat" v-model="repeat" type="password" autocomplete="new-password" /></div>
      </form>
      <DialogFooter>
        <Button variant="outline" @click="onOpenChange(false)">Cancel</Button>
        <Button type="submit" form="change-password" :disabled="busy">Change password</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
