// The idle warning dialog: two minutes before expiry, Stay signed in or
// Sign out. Dismissing without choosing stands down; the sign-in note
// explains if the session lapses afterwards.
<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import { giveUpSession, idleWarnBusy, idleWarnError, idleWarnOpen, noteAlive, staySignedIn } from '../idleWarning'

function closed() {
  noteAlive()
}
</script>

<template>
  <ResponsiveDialog
    :open="idleWarnOpen"
    title="Stay signed in?"
    description="Your session ends in about 2 minutes."
    :dismissible="!idleWarnBusy"
    @update:open="(next: boolean) => { if (!next) closed() }"
  >
    <p v-if="idleWarnError" class="text-sm text-destructive" role="alert">{{ idleWarnError }}</p>
    <template #footer>
      <ActionButton kind="secondary" label="Sign out" icon="log-out" :busy="idleWarnBusy" @press="giveUpSession" />
      <ActionButton kind="primary" label="Stay signed in" icon="check" :busy="idleWarnBusy" @press="staySignedIn" />
    </template>
  </ResponsiveDialog>
</template>
