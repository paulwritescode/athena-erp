// PermissionDenied covers a page the signed-in user must not see (reached
// directly by address). Hidden nav items keep this rare; the server still
// checks every call.
<script setup lang="ts">
import { Lock } from '@/icons'
import ActionButton from '@/components/ActionButton.vue'

defineProps<{ pageTitle: string }>()
const emit = defineEmits<{ goHome: [] }>()
</script>

<template>
  <div class="flex flex-col items-center px-6 py-12 text-center">
    <div class="bg-muted flex size-10 items-center justify-center rounded-lg border">
      <Lock class="text-muted-foreground size-5" aria-hidden="true" />
    </div>
    <h2 class="mt-4">You don't have access to {{ pageTitle }}</h2>
    <p class="text-muted-foreground mt-1 max-w-sm text-sm">Ask your administrator for access if you need this page.</p>
    <div class="mt-4">
      <ActionButton kind="secondary" label="Back to Overview" icon="house" @press="emit('goHome')" />
    </div>
  </div>
</template>
