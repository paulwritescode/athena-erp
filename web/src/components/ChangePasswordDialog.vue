<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { authClient } from '../api'
import ActionButton from '@/components/ActionButton.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import TextField from '@/components/fields/TextField.vue'
import { toast } from 'vue-sonner'

// The server applies the password policy (12 to 256 characters, not the
// username) and ends the user's other sessions.
const open = defineModel<boolean>('open', { required: true })
const current = ref('')
const next = ref('')
const repeat = ref('')
const busy = ref(false)
const dirty = computed(() => current.value !== '' || next.value !== '' || repeat.value !== '')

function reset() {
  current.value = ''
  next.value = ''
  repeat.value = ''
}

watch(open, (isOpen) => {
  if (!isOpen) reset()
})

async function change() {
  if (next.value !== repeat.value) {
    toast.error('The new passwords do not match.')
    return
  }
  busy.value = true
  try {
    await authClient.changePassword({ currentPassword: current.value, newPassword: next.value })
    toast.success('Password changed. Your other sessions were signed out.')
    open.value = false
  } catch (err) {
    toast.error(err instanceof ConnectError ? `Could not change the password: ${err.rawMessage}` : 'Could not change the password.')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <ResponsiveDialog
    v-model:open="open"
    title="Change password"
    description="Use at least 12 characters. Your other signed-in sessions end."
    :dirty="dirty && !busy"
    :dismissible="!busy"
  >
    <form id="change-password" class="sign-in-form" @submit.prevent="change">
      <TextField id="cp-current" label="Current password" type="password" autocomplete="current-password" v-model="current" :disabled="busy" />
      <TextField id="cp-new" label="New password" type="password" autocomplete="new-password" v-model="next" :disabled="busy" />
      <TextField id="cp-repeat" label="Repeat the new password" type="password" autocomplete="new-password" v-model="repeat" :disabled="busy" />
    </form>
    <template #footer>
      <ActionButton kind="secondary" label="Cancel" icon="x" :busy="busy" @press="open = false" />
      <ActionButton type="submit" form="change-password" kind="primary" label="Change password" icon="key-round" :busy="busy" />
    </template>
  </ResponsiveDialog>
</template>
