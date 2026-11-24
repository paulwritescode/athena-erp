// One builder row: field → operator → value(s). Emits a new condition
// object on every change; the group owns the tree.
<script setup lang="ts">
import { computed } from 'vue'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { OPERATOR_LABELS, OPERATORS_FOR_TYPE, type BuilderField, type ConditionOp, type TreeCondition } from '@/conditions'
import { Plus, X } from '@/icons'

const props = defineProps<{
  condition: TreeCondition
  fields: BuilderField[]
  rowId: string
}>()

const emit = defineEmits<{
  update: [condition: TreeCondition]
  remove: []
}>()

const field = computed(() => props.fields.find((f) => f.name === props.condition.field) ?? null)
const operators = computed<ConditionOp[]>(() => {
  const list = field.value ? OPERATORS_FOR_TYPE[field.value.type] : undefined
  return list ?? OPERATORS_FOR_TYPE['Text'] ?? []
})

function setField(name: string) {
  const next = props.fields.find((f) => f.name === name)
  const ops = (next ? OPERATORS_FOR_TYPE[next.type] : undefined) ?? OPERATORS_FOR_TYPE['Text'] ?? []
  emit('update', { field: name, op: ops[0] ?? 'is', values: [] })
}

function setOp(op: ConditionOp) {
  emit('update', { ...props.condition, op, values: [] })
}

function onOpInput(value: unknown) {
  setOp(String(value) as ConditionOp)
}

function setValue(index: number, value: string) {
  const values = [...(props.condition.values ?? [])]
  values[index] = value
  emit('update', { ...props.condition, values })
}

function addValue() {
  emit('update', { ...props.condition, values: [...(props.condition.values ?? []), ''] })
}

function removeValue(index: number) {
  emit('update', { ...props.condition, values: (props.condition.values ?? []).filter((_, i) => i !== index) })
}

const needsNoValue = computed(() => ['is_empty', 'is_not_empty', 'is_checked', 'is_not_checked'].includes(props.condition.op))
const needsManyValues = computed(() => ['is_any_of', 'is_none_of'].includes(props.condition.op))
const needsDayCount = computed(() => ['in_the_last_n_days', 'in_the_next_n_days'].includes(props.condition.op))
const valuePlaceholder = computed(() => {
  if (needsDayCount.value) return '30'
  if (field.value?.kind === 'date') return 'YYYY-MM-DD'
  if (field.value?.kind === 'decimal' || field.value?.kind === 'money' || field.value?.kind === 'int') return '0'
  return 'Value'
})
const valueInputmode = computed(() => {
  if (needsDayCount.value) return 'numeric'
  if (field.value?.kind === 'decimal' || field.value?.kind === 'money' || field.value?.kind === 'int') return 'decimal'
  return undefined
})
const choicesId = computed(() => `${props.rowId}-choices`)
const showChoices = computed(() => (field.value?.choices?.length ?? 0) > 0 && !needsManyValues.value && !needsNoValue.value)
</script>

<template>
  <div class="flex flex-wrap items-center gap-2" data-testid="condition-row">
    <NativeSelect :model-value="condition.field" size="sm" :aria-label="`Field for condition`" @update:model-value="setField(String($event))">
      <NativeSelectOption v-for="f in fields" :key="f.name" :value="f.name">{{ f.label }}</NativeSelectOption>
    </NativeSelect>
    <NativeSelect :model-value="condition.op" size="sm" aria-label="Comparison" @update:model-value="onOpInput($event)">
      <NativeSelectOption v-for="op in operators" :key="op" :value="op">{{ OPERATOR_LABELS[op] }}</NativeSelectOption>
    </NativeSelect>
    <template v-if="!needsNoValue">
      <template v-if="needsManyValues">
        <div class="flex flex-col gap-1">
          <div v-for="(_, index) in (condition.values ?? [''])" :key="index" class="flex items-center gap-1">
            <Input :model-value="(condition.values ?? [])[index] ?? ''" class="h-8" :aria-label="`Value ${index + 1}`" :placeholder="valuePlaceholder" @update:model-value="setValue(index, String($event))" />
            <button type="button" class="text-muted-foreground hover:text-foreground inline-flex size-7 items-center justify-center rounded-md" :aria-label="`Remove value ${index + 1}`" @click="removeValue(index)">
              <X class="size-4" aria-hidden="true" />
            </button>
          </div>
          <button type="button" class="text-muted-foreground hover:text-foreground inline-flex h-7 items-center gap-1 text-sm" @click="addValue">
            <Plus class="size-4" aria-hidden="true" /> Add value
          </button>
        </div>
      </template>
      <template v-else>
        <Input :model-value="(condition.values ?? [])[0] ?? ''" class="h-8" aria-label="Value" :placeholder="valuePlaceholder" :inputmode="valueInputmode" :list="showChoices ? choicesId : undefined" @update:model-value="setValue(0, String($event))" />
        <datalist v-if="showChoices" :id="choicesId">
          <option v-for="choice in field?.choices ?? []" :key="choice" :value="choice" />
        </datalist>
        <span v-if="needsDayCount" class="text-muted-foreground text-sm">days</span>
      </template>
    </template>
    <button type="button" class="text-muted-foreground hover:text-foreground inline-flex size-7 items-center justify-center rounded-md" aria-label="Remove condition" @click="emit('remove')">
      <X class="size-4" aria-hidden="true" />
    </button>
  </div>
</template>
