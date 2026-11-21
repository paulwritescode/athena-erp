// FieldShell is the shared chrome for §7 form fields: label above, the
// control, help in muted caption or error in destructive with an alert icon.
// Required is the default and unmarked; optional fields say so.
<script setup lang="ts">
import { computed } from 'vue'
import { CircleAlert } from '@/icons'
import { Label } from '@/components/ui/label'

const props = defineProps<{ id: string; label: string; optional?: boolean; help?: string; error?: string }>()

const describedBy = computed(() => {
  if (props.error) return `${props.id}-error`
  if (props.help) return `${props.id}-help`
  return undefined
})
</script>

<template>
  <div class="field-shell">
    <Label :for="id">{{ label }}<span v-if="optional"> (optional)</span></Label>
    <slot :describedby="describedBy" :invalid="!!error" />
    <p v-if="error" :id="`${id}-error`" class="field-error"><CircleAlert class="size-4 shrink-0" aria-hidden="true" /><span>{{ error }}</span></p>
    <p v-else-if="help" :id="`${id}-help`" class="field-help">{{ help }}</p>
  </div>
</template>
