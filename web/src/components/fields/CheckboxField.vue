// CheckboxField is the shared §7 yes-or-no input: the label above, a
// clickable box with the current answer beside it.
<script setup lang="ts">
import { Checkbox } from '@/components/ui/checkbox'
import FieldShell from './FieldShell.vue'

withDefaults(defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  onLabel?: string
  offLabel?: string
  modelValue?: boolean
}>(), { onLabel: 'Yes', offLabel: 'No' })
defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <div class="flex min-h-11 items-center gap-2 min-[640px]:min-h-10 lg:min-h-8">
      <Checkbox
        :id="id"
        :model-value="modelValue ?? false"
        :disabled="disabled"
        @update:model-value="$emit('update:modelValue', $event === true)"
      />
      <span aria-hidden="true">{{ modelValue ? onLabel : offLabel }}</span>
    </div>
  </FieldShell>
</template>
