// LongTextField is the shared §7 multi-line input.
<script setup lang="ts">
import { Textarea } from '@/components/ui/textarea'
import FieldShell from './FieldShell.vue'

withDefaults(defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  rows?: number
  modelValue?: string
}>(), { rows: 4 })
defineEmits<{ 'update:modelValue': [value: string]; blur: [] }>()
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <Textarea
        :id="id"
        :model-value="modelValue ?? ''"
        :rows="rows"
        :placeholder="placeholder"
        :disabled="disabled"
        :aria-invalid="invalid"
        :aria-required="!optional || undefined"
        :aria-describedby="describedby"
        @update:model-value="$emit('update:modelValue', String($event))"
        @blur="$emit('blur')"
      />
    </template>
  </FieldShell>
</template>
