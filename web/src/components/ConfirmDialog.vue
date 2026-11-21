// ConfirmDialog asks before a destructive action: what goes away, and a
// clear way out. The confirm button reads as the action, not "OK".
<script setup lang="ts">
import { LoaderCircle } from '@lucide/vue'
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'

const props = withDefaults(defineProps<{ title: string; body: string; confirmLabel?: string; cancelLabel?: string; danger?: boolean; busy?: boolean; disabledReason?: string | null }>(), {
  confirmLabel: 'Delete',
  cancelLabel: 'Cancel',
  danger: true,
  busy: false,
  disabledReason: null,
})
const emit = defineEmits<{ confirm: [] }>()
const open = defineModel<boolean>('open', { required: true })

function confirmed() {
  if (open.value) emit('confirm')
}

// While busy the dialog stays put: Esc and outside clicks wait for the
// action to finish.
function guardDismiss(event: Event) {
  if (props.busy) event.preventDefault()
}
</script>

<template>
  <AlertDialog v-model:open="open">
    <AlertDialogContent @escape-key-down="guardDismiss" @pointer-down-outside="guardDismiss">
      <AlertDialogHeader>
        <AlertDialogTitle>{{ title }}</AlertDialogTitle>
        <AlertDialogDescription>{{ body }}</AlertDialogDescription>
      </AlertDialogHeader>
      <p v-if="disabledReason && !busy" class="text-muted-foreground text-xs">{{ disabledReason }}</p>
      <AlertDialogFooter>
        <AlertDialogCancel :disabled="busy">{{ cancelLabel }}</AlertDialogCancel>
        <Button :variant="danger ? 'destructive' : 'default'" :disabled="busy || disabledReason !== null" @click="confirmed">
          <LoaderCircle v-if="busy" class="size-4 animate-spin" aria-hidden="true" />
          {{ confirmLabel }}
        </Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
