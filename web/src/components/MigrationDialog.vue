<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { FieldValueSchema, type FieldValue } from '../gen/athena/v1/entries_pb'
import type { SaveAnswers, SavePlan } from '../api'
import { validateField } from '../fieldRules'
import type { Field, RecordDefinition } from '../types'
import { Button } from '@/components/ui/button'
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'

// plan is null when the dialog is closed.
const props = defineProps<{ record: RecordDefinition; plan: SavePlan | null }>()
const emit = defineEmits<{ confirm: [answers: SaveAnswers]; cancel: [] }>()

const texts = reactive<Record<string, string>>({})
const checks = reactive<Record<string, boolean>>({})
const errors = reactive<Record<string, string>>({})
const accepted = ref(false)
// Opening the dialog moves focus, which blurs inputs; only check what was typed in.
const touched = new Set<string>()

const destructive = computed(() => props.plan?.steps.some((step) => step.destructive) ?? false)
const backfillFields = computed<Field[]>(() => {
  const keys = new Set(props.plan?.steps.filter((step) => step.needsBackfill).map((step) => step.fieldKey) ?? [])
  return props.record.fields.filter((field) => keys.has(field.name))
})

watch(() => props.plan, () => {
  for (const target of [texts, checks, errors]) for (const key of Object.keys(target)) delete target[key]
  accepted.value = false
  touched.clear()
})

function check(field: Field) {
  const problem = validateField({ ...field, required: true }, texts[field.name] ?? '')
  if (problem) errors[field.name] = problem
  else delete errors[field.name]
}

function confirm() {
  for (const field of backfillFields.value) if (field.type !== 'Checkbox') check(field)
  if (Object.keys(errors).length > 0 || (destructive.value && !accepted.value)) return
  const backfills: { [key: string]: FieldValue } = {}
  for (const field of backfillFields.value) {
    backfills[field.name] = field.type === 'Checkbox'
      ? create(FieldValueSchema, { kind: { case: 'boolValue', value: checks[field.name] ?? false } })
      : create(FieldValueSchema, { kind: { case: 'stringValue', value: (texts[field.name] ?? '').trim() } })
  }
  emit('confirm', { confirmDataLoss: destructive.value, backfills })
}

function onOpenChange(open: boolean) {
  if (!open) emit('cancel')
}

function inputType(field: Field) {
  return field.type === 'Email' ? 'email' : field.type === 'Date' ? 'date' : field.type === 'Phone' ? 'tel' : 'text'
}
</script>

<template>
  <AlertDialog :open="plan !== null" @update:open="onOpenChange">
    <AlertDialogContent class="migration-dialog">
      <AlertDialogHeader>
        <AlertDialogTitle>Review changes to {{ record.label }}</AlertDialogTitle>
        <AlertDialogDescription>Saving changes the table for this Record and its existing entries.</AlertDialogDescription>
      </AlertDialogHeader>
      <ul class="migration-steps">
        <li v-for="step in plan?.steps ?? []" :key="`${step.kind}:${step.fieldKey}`" :class="{ 'is-destructive': step.destructive }">{{ step.description }}.</li>
      </ul>
      <div v-if="backfillFields.length > 0" class="migration-backfills">
        <p class="migration-note">Fill-in value for entries without one</p>
        <div v-for="field in backfillFields" :key="field.name" class="form-field" :class="{ 'is-invalid': errors[field.name] }">
          <label :for="`backfill-${field.name}`" class="form-label">{{ field.label }}</label>
          <label v-if="field.type === 'Checkbox'" class="checkbox-row"><input :id="`backfill-${field.name}`" v-model="checks[field.name]" type="checkbox" /><span>{{ checks[field.name] ? 'Yes' : 'No' }}</span></label>
          <input v-else :id="`backfill-${field.name}`" v-model="texts[field.name]" :type="inputType(field)" :aria-invalid="errors[field.name] ? 'true' : undefined" @blur="touched.has(field.name) && check(field)" @input="touched.add(field.name); errors[field.name] && check(field)" />
          <p v-if="errors[field.name]" class="field-error">{{ errors[field.name] }}</p>
        </div>
      </div>
      <label v-if="destructive" class="migration-accept"><input v-model="accepted" type="checkbox" />I understand the values listed above are deleted and cannot be recovered.</label>
      <AlertDialogFooter>
        <AlertDialogCancel>Cancel</AlertDialogCancel>
        <Button :variant="destructive ? 'destructive' : 'default'" :disabled="destructive && !accepted" @click="confirm">Save changes</Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
