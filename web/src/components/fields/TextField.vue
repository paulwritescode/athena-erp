// TextField is the shared §7 text input: stacked label, control at the §7
// heights (44 phone, 40 tablet, 36 desktop), help or error below.
<script setup lang="ts">
import { Input } from '@/components/ui/input'
import FieldShell from './FieldShell.vue'

withDefaults(defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  type?: string
  inputmode?: 'decimal' | 'email' | 'none' | 'numeric' | 'search' | 'tel' | 'text' | 'url'
  autocomplete?: string
  autofocus?: boolean
  spellcheck?: boolean
  modelValue?: string
}>(), { type: 'text' })
defineEmits<{ 'update:modelValue': [value: string]; blur: []; enter: [] }>()
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <Input
        :id="id"
        :model-value="modelValue ?? ''"
        :type="type"
        :inputmode="inputmode"
        :autocomplete="autocomplete"
        :placeholder="placeholder"
        :autofocus="autofocus"
        :spellcheck="spellcheck"
        :disabled="disabled"
        :aria-invalid="invalid"
        :aria-required="!optional || undefined"
        :aria-describedby="describedby"
        class="h-11 min-[640px]:h-10 lg:h-9"
        @update:model-value="$emit('update:modelValue', String($event))"
        @blur="$emit('blur')"
        @keyup.enter="$emit('enter')"
      />
    </template>
  </FieldShell>
</template>
