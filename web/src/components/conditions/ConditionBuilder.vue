// ConditionBuilder: the F4 no-code condition editor. Edits a condition tree
// over the given fields, compiles it live through conditions.ts, and shows
// the expression (or the first problem) on request. Hosts: the Rules page,
// F3 field conditions, the F2 advanced filter.
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { compileTree, type BuilderField, type ConditionTree, type TreeField } from '@/conditions'
import ConditionGroup from './ConditionGroup.vue'

const props = defineProps<{
  modelValue: ConditionTree | null
  fields: BuilderField[]
  compact?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [tree: ConditionTree]
}>()

function blankTree(): ConditionTree {
  return { root: { all: true, conditions: [] } }
}

function clone(tree: ConditionTree | null): ConditionTree {
  // JSON round-trip: the tree model is JSON by design, and it detaches
  // reactive proxies (which structuredClone refuses).
  return JSON.parse(JSON.stringify(tree ?? blankTree())) as ConditionTree
}

// Local draft with JSON-equality guards both ways, so host echoes never loop.
const draft = ref<ConditionTree>(clone(props.modelValue))
watch(() => props.modelValue, (next) => {
  if (JSON.stringify(next ?? null) !== JSON.stringify(draft.value)) draft.value = clone(next)
}, { deep: true })
watch(draft, (next) => {
  if (JSON.stringify(next) !== JSON.stringify(props.modelValue ?? null)) emit('update:modelValue', clone(next))
}, { deep: true })

const showExpression = ref(false)

const compiled = computed<{ expr: string; error: string }>(() => {
  const fields: Record<string, TreeField> = {}
  for (const f of props.fields) fields[f.name] = { kind: f.kind, currency: f.currency, scale: f.scale }
  try {
    return { expr: compileTree(draft.value, fields), error: '' }
  } catch (err) {
    return { expr: '', error: err instanceof Error ? err.message : String(err) }
  }
})

function updateRoot(group: ConditionTree['root']) {
  draft.value = { root: group }
}

const isEmpty = computed(() => (draft.value.root.conditions ?? []).length + (draft.value.root.groups ?? []).length === 0)
</script>

<template>
  <div class="space-y-3" data-testid="condition-builder">
    <p v-if="isEmpty" class="text-muted-foreground text-sm">
      No conditions yet — the rule matches every record. Add one to narrow it down.
    </p>
    <ConditionGroup :group="draft.root" :fields="fields" group-id="builder-root" :is-root="true" :compact="compact" @update="updateRoot" />
    <div>
      <button type="button" class="text-muted-foreground hover:text-foreground inline-flex h-7 items-center gap-1 text-sm" :aria-expanded="showExpression" @click="showExpression = !showExpression">
        {{ showExpression ? 'Hide expression' : 'Show as expression' }}
      </button>
      <p v-if="showExpression && compiled.expr" class="bg-muted mt-1 rounded-md p-2 font-mono text-xs break-all" data-testid="builder-expression">{{ compiled.expr }}</p>
      <p v-if="compiled.error && !isEmpty" class="text-destructive mt-1 text-sm" role="alert" data-testid="builder-error">{{ compiled.error }}</p>
    </div>
  </div>
</template>
