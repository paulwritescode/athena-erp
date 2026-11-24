// Search input for the shared table (F2): the box shows every keystroke at
// once while the table hears about it after a pause; clearing applies at
// once. The caller owns the value (it shapes the query) through v-model.
<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { Search, X } from '@/icons'
import { Input } from '@/components/ui/input'
import { gloss } from '@/copy/glossary'
import { SEARCH_DEBOUNCE_MS } from './dataTableSearch'

const props = withDefaults(defineProps<{
  modelValue?: string
  placeholder?: string
  debounceMs?: number
}>(), { modelValue: '', placeholder: '', debounceMs: SEARCH_DEBOUNCE_MS })
const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const text = ref(props.modelValue)
let timer: ReturnType<typeof setTimeout> | undefined
onUnmounted(() => clearTimeout(timer))
// The caller's value wins (clearing elsewhere, switching lists): it cancels
// a pending burst and shows at once.
watch(() => props.modelValue, (value) => {
  clearTimeout(timer)
  text.value = value
})
function onInput(value: string | number) {
  text.value = String(value)
  clearTimeout(timer)
  timer = setTimeout(() => emit('update:modelValue', text.value), props.debounceMs)
}
function clear() {
  clearTimeout(timer)
  text.value = ''
  emit('update:modelValue', '')
}
</script>

<template>
  <div class="relative w-full min-w-40 flex-1 sm:max-w-[480px]">
    <Search class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" aria-hidden="true" />
    <Input :model-value="text" type="text" :placeholder="placeholder" :aria-label="placeholder" class="pr-8! pl-9!" @update:model-value="onInput" />
    <button
      v-if="text"
      type="button"
      :aria-label="gloss('clearSearch').label"
      class="text-muted-foreground hover:text-foreground absolute top-1/2 right-2 grid size-6 -translate-y-1/2 place-items-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring"
      @click="clear"
    >
      <X class="size-3.5" aria-hidden="true" />
    </button>
  </div>
</template>
