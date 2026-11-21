// LinkField is the shared §7 linked-record input: the §7 chrome around the
// server-searching RecordPicker. The model is the linked Record ID.
<script setup lang="ts">
import RecordPicker from '@/components/RecordPicker.vue'
import FieldShell from './FieldShell.vue'

defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  recordTypeKey: string
  targetLabel: string
  modelValue?: string
}>()
defineEmits<{ 'update:modelValue': [value: string]; blur: [] }>()
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <RecordPicker
        :id="id"
        :model-value="modelValue ?? ''"
        :record-type-key="recordTypeKey"
        :target-label="targetLabel"
        :required="!optional"
        :disabled="disabled"
        :invalid="invalid"
        :describedby="describedby"
        @update:model-value="$emit('update:modelValue', String($event))"
        @blur="$emit('blur')"
      />
    </template>
  </FieldShell>
</template>
