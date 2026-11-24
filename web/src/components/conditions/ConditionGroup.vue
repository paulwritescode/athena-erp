// One condition group: a match-all/any connector over condition rows and
// nested groups. Recurses by filename for nesting.
<script setup lang="ts">
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import type { BuilderField, TreeCondition, TreeGroup } from '@/conditions'
import { Plus, X } from '@/icons'
import ConditionRow from './ConditionRow.vue'

const props = defineProps<{
  group: TreeGroup
  fields: BuilderField[]
  groupId: string
  isRoot?: boolean
  compact?: boolean
}>()

const emit = defineEmits<{
  update: [group: TreeGroup]
  remove: []
}>()

function setConnector(value: string) {
  emit('update', { ...props.group, all: value === 'all' })
}

function updateCondition(index: number, condition: TreeCondition) {
  const conditions = [...(props.group.conditions ?? [])]
  conditions[index] = condition
  emit('update', { ...props.group, conditions })
}

function removeCondition(index: number) {
  emit('update', { ...props.group, conditions: (props.group.conditions ?? []).filter((_, i) => i !== index) })
}

function updateGroup(index: number, group: TreeGroup) {
  const groups = [...(props.group.groups ?? [])]
  groups[index] = group
  emit('update', { ...props.group, groups })
}

function removeGroup(index: number) {
  emit('update', { ...props.group, groups: (props.group.groups ?? []).filter((_, i) => i !== index) })
}

function defaultCondition(): TreeCondition {
  const first = props.fields[0]
  return { field: first?.name ?? '', op: 'is', values: [] }
}

function addCondition() {
  emit('update', { ...props.group, conditions: [...(props.group.conditions ?? []), defaultCondition()] })
}

function addGroup() {
  emit('update', { ...props.group, groups: [...(props.group.groups ?? []), { all: true, conditions: [defaultCondition(), defaultCondition()] }] })
}
</script>

<template>
  <div class="space-y-2" :class="{ 'border-border rounded-lg border p-3': !isRoot }" data-testid="condition-group">
    <div class="flex items-center gap-2">
      <NativeSelect :model-value="group.all ? 'all' : 'any'" size="sm" aria-label="Match all or any" @update:model-value="setConnector(String($event))">
        <NativeSelectOption value="all">Match all of</NativeSelectOption>
        <NativeSelectOption value="any">Match any of</NativeSelectOption>
      </NativeSelect>
      <button v-if="!isRoot" type="button" class="text-muted-foreground hover:text-foreground inline-flex size-7 items-center justify-center rounded-md" aria-label="Remove group" @click="emit('remove')">
        <X class="size-4" aria-hidden="true" />
      </button>
    </div>
    <ConditionRow
      v-for="(condition, index) in (group.conditions ?? [])"
      :key="`c${index}`"
      :condition="condition"
      :fields="fields"
      :row-id="`${groupId}-c${index}`"
      @update="updateCondition(index, $event)"
      @remove="removeCondition(index)"
    />
    <ConditionGroup
      v-for="(child, index) in (group.groups ?? [])"
      :key="`g${index}`"
      :group="child"
      :fields="fields"
      :group-id="`${groupId}-g${index}`"
      :compact="compact"
      @update="updateGroup(index, $event)"
      @remove="removeGroup(index)"
    />
    <div class="flex items-center gap-2">
      <button type="button" class="text-muted-foreground hover:text-foreground inline-flex h-7 items-center gap-1 text-sm" @click="addCondition">
        <Plus class="size-4" aria-hidden="true" /> Add condition
      </button>
      <button v-if="!compact" type="button" class="text-muted-foreground hover:text-foreground inline-flex h-7 items-center gap-1 text-sm" @click="addGroup">
        <Plus class="size-4" aria-hidden="true" /> Add group
      </button>
    </div>
  </div>
</template>
