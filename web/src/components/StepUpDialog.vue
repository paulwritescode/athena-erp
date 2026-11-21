// The step-up password dialog (B7, R7.3): when the server answers
// step_up_required, the api interceptor opens this once however many calls
// are waiting. A right password retries each original call once with the
// rotated session; cancelling returns every caller to its usual error.
<script setup lang="ts">
import { ref, watch } from 'vue'
import ActionButton from '@/components/ActionButton.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import TextField from '@/components/fields/TextField.vue'
import { cancelStepUp, stepUpBusy, stepUpError, stepUpOpen, submitStepUp } from '../stepUp'

const password = ref('')

watch(stepUpOpen, () => {
  // Never keep the secret: cleared on open and on close alike.
  password.value = ''
})

async function submit() {
  await submitStepUp(password.value)
  password.value = ''
}
</script>

<template>
  <ResponsiveDialog
    :open="stepUpOpen"
    title="Enter your password to continue"
    description="This action needs fresh proof it is you. Nothing is lost while you decide."
    @update:open="(next: boolean) => { if (!next) cancelStepUp() }"
  >
    <form id="step-up-form" @submit.prevent="submit">
      <TextField
        id="step-up-password"
        label="Your password"
        type="password"
        autocomplete="current-password"
        :error="stepUpError || undefined"
        :disabled="stepUpBusy"
        v-model="password"
      />
    </form>
    <template #footer>
      <ActionButton kind="secondary" label="Cancel" icon="x" :busy="stepUpBusy" @press="cancelStepUp" />
      <ActionButton type="submit" form="step-up-form" kind="primary" label="Continue" icon="key-round" :busy="stepUpBusy" />
    </template>
  </ResponsiveDialog>
</template>
