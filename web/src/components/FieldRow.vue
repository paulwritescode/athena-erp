// FieldRow lays out one labelled control: the label on the left, the
// control on the right, and the hint or error under the control. The label
// stacks above the control below 640 px.
<script setup lang="ts">
import { Label } from '@/components/ui/label'

defineProps<{ label: string; hint?: string; error?: string; required?: boolean; for?: string; stacked?: boolean }>()
</script>

<template>
  <div class="grid gap-2" :class="stacked ? undefined : 'min-[640px]:grid-cols-[180px_minmax(0,1fr)] min-[640px]:gap-4'">
    <Label :for="$props.for" :class="stacked ? undefined : 'min-[640px]:pt-2'">
      {{ label }}
      <span v-if="required" class="text-destructive ml-0.5" aria-hidden="true">*</span>
      <span v-if="required" class="sr-only">(required)</span>
    </Label>
    <div class="min-w-0">
      <slot />
      <p v-if="error" class="text-destructive mt-1.5 text-xs" role="alert">{{ error }}</p>
      <p v-else-if="hint" class="text-muted-foreground mt-1.5 text-xs">{{ hint }}</p>
    </div>
  </div>
</template>
