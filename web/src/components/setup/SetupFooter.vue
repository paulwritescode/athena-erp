// Shared footer for every Setup wizard step (B11 slice 2): Back on the
// left, quiet actions and the one primary on the right, so the same
// action sits in the same place on every step (§7, §14).
<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import type { IconName } from '@/icons'

withDefaults(
  defineProps<{
    showBack?: boolean
    primaryLabel: string
    primaryIcon?: IconName
    primaryBusy?: boolean
    primaryDisabledReason?: string | null
  }>(),
  { showBack: true, primaryIcon: 'chevron-right', primaryBusy: false, primaryDisabledReason: null },
)
const emit = defineEmits<{ back: []; primary: [] }>()
</script>

<template>
  <div class="mt-6 flex flex-wrap items-center justify-between gap-2 border-t pt-6">
    <ActionButton v-if="showBack" kind="secondary" label="Back" icon="arrow-left" @press="emit('back')" />
    <span v-else />
    <div class="flex flex-wrap items-center gap-2">
      <slot />
      <ActionButton
        kind="primary"
        :label="primaryLabel"
        :icon="primaryIcon"
        :busy="primaryBusy"
        :disabled-reason="primaryDisabledReason ?? null"
        @press="emit('primary')"
      />
    </div>
  </div>
</template>
