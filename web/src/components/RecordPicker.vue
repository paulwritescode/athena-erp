<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { ConnectError } from '@connectrpc/connect'
import { ChevronsUpDown } from '@/icons'
import { toast } from 'vue-sonner'
import type { LookupItem } from '../gen/athena/v1/records_pb'
import { labelWithState, resolveRecords, searchRecords } from '../lookup'
import BottomDrawer from '@/components/BottomDrawer.vue'
import RecordPickerList from '@/components/RecordPickerList.vue'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

// Chooses a Record of one Record Type by its label. The value is the Record ID;
// people never see or type it. The list is searched on the server, so it
// holds only Records that can be chosen now.
const props = defineProps<{
  modelValue: string
  recordTypeKey: string
  /** What the Record Type is called, for the placeholder and the empty state. */
  targetLabel: string
  id?: string
  disabled?: boolean
  required?: boolean
  invalid?: boolean
  describedby?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [id: string]; blur: [] }>()

// Phones and tablets pick from a bottom drawer (§13.2); desktop keeps the popover.
const isDesktop = useMediaQuery('(min-width: 1024px)')
const open = ref(false)
const query = ref('')
const items = ref<LookupItem[]>([])
const nextPageToken = ref('')
const loading = ref(false)
const selected = ref<LookupItem | null>(null)
let request = 0
let timer: ReturnType<typeof setTimeout> | undefined
// The text of the last search, so resetting the field on open does not search twice.
let searched = ''

function message(err: unknown) {
  return err instanceof ConnectError ? err.rawMessage : 'Something went wrong.'
}

// Labels the current value, including one whose Record was cancelled or deleted.
async function showSelected() {
  if (!props.modelValue) { selected.value = null; return }
  const id = props.modelValue
  try {
    const found = (await resolveRecords(props.recordTypeKey, [id])).get(id)
    if (props.modelValue === id) selected.value = found ?? null
  } catch (err) {
    toast.error(`Could not load the ${props.targetLabel}: ${message(err)}`)
  }
}
watch(() => [props.modelValue, props.recordTypeKey] as const, () => { void showSelected() }, { immediate: true })

async function search(more = false) {
  const current = ++request
  if (!more) searched = query.value
  loading.value = true
  try {
    const res = await searchRecords(props.recordTypeKey, query.value, more ? nextPageToken.value : '')
    if (current !== request) return
    items.value = more ? [...items.value, ...res.items] : res.items
    nextPageToken.value = res.nextPageToken
  } catch (err) {
    if (current !== request) return
    toast.error(`Could not search ${props.targetLabel} records: ${message(err)}`)
  } finally {
    if (current === request) loading.value = false
  }
}

watch(open, (isOpen) => {
  if (isOpen) { clearTimeout(timer); query.value = ''; items.value = []; nextPageToken.value = ''; void search() }
  else emit('blur')
})
// Typing asks the server again after a short pause. Opening resets the text
// and searches at once, so only typing waits.
watch(query, () => {
  if (!open.value || query.value === searched) return
  clearTimeout(timer)
  timer = setTimeout(() => { void search() }, 200)
})
onBeforeUnmount(() => clearTimeout(timer))

function choose(item: LookupItem | null) {
  selected.value = item
  emit('update:modelValue', item?.id ?? '')
  open.value = false
}
</script>

<template>
  <Popover v-if="isDesktop" v-model:open="open">
    <PopoverTrigger as-child>
      <Button :id="id" type="button" variant="outline" role="combobox" class="picker-trigger" :disabled="disabled" :aria-expanded="open" :aria-invalid="invalid ? 'true' : undefined" :aria-required="required || undefined" :aria-describedby="describedby">
        <span :class="selected ? undefined : 'muted-label'">{{ selected ? labelWithState(selected) : `Choose a ${targetLabel}…` }}</span>
        <ChevronsUpDown class="picker-icon" aria-hidden="true" />
      </Button>
    </PopoverTrigger>
    <PopoverContent class="picker-content" align="start">
      <RecordPickerList
        v-model:query="query"
        :target-label="targetLabel"
        :items="items"
        :loading="loading"
        :next-page-token="nextPageToken"
        :model-value="modelValue"
        :can-clear="!required && modelValue !== ''"
        @select="choose"
        @more="search(true)"
      />
    </PopoverContent>
  </Popover>
  <template v-else>
    <Button :id="id" type="button" variant="outline" role="combobox" class="picker-trigger" :disabled="disabled" :aria-expanded="open" :aria-invalid="invalid ? 'true' : undefined" :aria-required="required || undefined" :aria-describedby="describedby" @click="open = true">
      <span :class="selected ? undefined : 'muted-label'">{{ selected ? labelWithState(selected) : `Choose a ${targetLabel}…` }}</span>
      <ChevronsUpDown class="picker-icon" aria-hidden="true" />
    </Button>
    <BottomDrawer v-model:open="open" :title="`Choose a ${targetLabel}`">
      <RecordPickerList
        v-model:query="query"
        :target-label="targetLabel"
        :items="items"
        :loading="loading"
        :next-page-token="nextPageToken"
        :model-value="modelValue"
        :can-clear="!required && modelValue !== ''"
        @select="choose"
        @more="search(true)"
      />
    </BottomDrawer>
  </template>
</template>
