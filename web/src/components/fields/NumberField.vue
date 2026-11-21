// NumberField is the shared §7 numeric input: plain text with a decimal
// keypad on touch, so values keep their exact decimals (no number spinners,
// no locale surprises).
<script setup lang="ts">
import { Input } from '@/components/ui/input'
import FieldShell from './FieldShell.vue'

defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  autocomplete?: string
  modelValue?: string
}>()
defineEmits<{ 'update:modelValue': [value: string]; blur: [] }>()
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <Input
        :id="id"
        :model-value="modelValue ?? ''"
        type="text"
        inputmode="decimal"
        :autocomplete="autocomplete"
        :placeholder="placeholder"
        :disabled="disabled"
        :aria-invalid="invalid"
        :aria-required="!optional || undefined"
        :aria-describedby="describedby"
        class="h-11 min-[640px]:h-10 lg:h-9"
        @update:model-value="$emit('update:modelValue', String($event))"
        @blur="$emit('blur')"
      />
    </template>
  </FieldShell>
</template>
