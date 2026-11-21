// ConfirmDialog asks before a destructive action: what goes away, and a
// clear way out. The confirm button reads as the action, not "OK". While
// busy the dialog stays put: Esc, outside clicks and the close button wait
// for the action to finish.
<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import type { IconName } from '@/icons'

const props = withDefaults(defineProps<{
  title: string
  body: string
  confirmLabel?: string
  cancelLabel?: string
  confirmIcon?: IconName
  danger?: boolean
  busy?: boolean
  disabledReason?: string | null
}>(), {
  confirmLabel: 'Delete',
  cancelLabel: 'Cancel',
  confirmIcon: undefined,
  danger: true,
  busy: false,
  disabledReason: null,
})
const emit = defineEmits<{ confirm: [] }>()
const open = defineModel<boolean>('open', { required: true })

function confirmed() {
  if (open.value) emit('confirm')
}
</script>

<template>
  <ResponsiveDialog v-model:open="open" :title="title" :description="body" :dismissible="!busy">
    <p v-if="disabledReason && !busy" class="text-muted-foreground text-xs">{{ disabledReason }}</p>
    <template #footer>
      <ActionButton kind="secondary" :label="cancelLabel" icon="x" :busy="busy" @press="open = false" />
      <ActionButton
        :kind="danger ? 'destructive' : 'primary'"
        :label="confirmLabel"
        :icon="confirmIcon ?? (danger ? 'trash-2' : 'check')"
        :busy="busy"
        :disabled-reason="disabledReason"
        @press="confirmed"
      />
    </template>
  </ResponsiveDialog>
</template>
