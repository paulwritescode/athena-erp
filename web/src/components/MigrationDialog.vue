<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { FieldValueSchema, type FieldValue } from '../gen/athena/v1/records_pb'
import type { SaveAnswers, SavePlan } from '../api'
import { validateField } from '../fieldRules'
import type { Field, RecordType } from '../types'
import ActionButton from '@/components/ActionButton.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import CheckboxField from '@/components/fields/CheckboxField.vue'
import TextField from '@/components/fields/TextField.vue'

// plan is null when the dialog is closed.
const props = defineProps<{ recordType: RecordType; plan: SavePlan | null }>()
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
  return props.recordType.fields.filter((field) => keys.has(field.name))
})
const draftDirty = computed(() =>
  Object.values(texts).some((value) => value.trim() !== '') || Object.values(checks).some(Boolean) || accepted.value,
)

// The draft survives closing and reopening the same plan: a reset happens
// only for a different plan (new signature) or after the save succeeds
// (the revision moves). A failed save keeps the revision, so the draft
// stays for the retry.
let planSignature: string | null = null
function resetDraft() {
  for (const target of [texts, checks, errors]) for (const key of Object.keys(target)) delete target[key]
  accepted.value = false
  touched.clear()
}

watch(() => props.plan, (plan) => {
  if (!plan) return
  // Steps carry bigint counts, which JSON.stringify rejects.
  const signature = `${props.recordType.name}:${JSON.stringify(plan.steps, (_, value) => (typeof value === 'bigint' ? String(value) : value))}`
  if (signature !== planSignature) {
    planSignature = signature
    resetDraft()
  }
})

watch(() => props.recordType.revision, () => {
  planSignature = null
  resetDraft()
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
  <ResponsiveDialog :open="plan !== null" :title="'Review changes to ' + recordType.label" description="Saving changes the table for this Record Type and its existing records." wide :dirty="draftDirty" @update:open="onOpenChange">
      <ul class="migration-steps">
        <li v-for="step in plan?.steps ?? []" :key="`${step.kind}:${step.fieldKey}`" :class="{ 'is-destructive': step.destructive }">{{ step.description }}.</li>
      </ul>
      <div v-if="backfillFields.length > 0" class="migration-backfills">
        <p class="migration-note">Fill-in value for records without one</p>
        <template v-for="field in backfillFields" :key="field.name">
          <CheckboxField
            v-if="field.type === 'Checkbox'"
            :id="`backfill-${field.name}`"
            :label="field.label"
            :error="errors[field.name]"
            v-model="checks[field.name]"
          />
          <TextField
            v-else
            :id="`backfill-${field.name}`"
            :label="field.label"
            :error="errors[field.name]"
            v-model="texts[field.name]"
            :type="inputType(field)"
            @update:model-value="touched.add(field.name); errors[field.name] && check(field)"
            @blur="touched.has(field.name) && check(field)"
          />
        </template>
      </div>
      <label v-if="destructive" class="migration-accept"><input v-model="accepted" type="checkbox" />I understand the values listed above are deleted and cannot be recovered.</label>
      <template #footer>
        <ActionButton kind="secondary" label="Cancel" icon="x" @press="onOpenChange(false)" />
        <ActionButton :kind="destructive ? 'destructive' : 'primary'" label="Save changes" icon="save" :disabled-reason="destructive && !accepted ? 'Confirm the data loss above first.' : null" @press="confirm" />
      </template>
  </ResponsiveDialog>
</template>
