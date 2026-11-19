<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { CheckIcon, ChevronsUpDownIcon, SearchIcon, XIcon } from '@lucide/vue'
import { toast } from 'vue-sonner'
import type { LookupItem } from '../gen/athena/v1/records_pb'
import { labelWithState, resolveRecords, searchRecords } from '../lookup'
import { Button } from '@/components/ui/button'
import { Command, CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { InputGroup, InputGroupAddon } from '@/components/ui/input-group'
import { ListboxFilter } from 'reka-ui'
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
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button :id="id" type="button" variant="outline" role="combobox" class="picker-trigger" :disabled="disabled" :aria-expanded="open" :aria-invalid="invalid ? 'true' : undefined" :aria-required="required || undefined" :aria-describedby="describedby">
        <span :class="selected ? undefined : 'muted-label'">{{ selected ? labelWithState(selected) : `Choose a ${targetLabel}…` }}</span>
        <ChevronsUpDownIcon class="picker-icon" aria-hidden="true" />
      </Button>
    </PopoverTrigger>
    <PopoverContent class="picker-content" align="start">
      <Command>
        <!-- The server filters, so the search text stays out of the Command's own client-side filter. -->
        <div class="p-1 pb-0">
          <InputGroup class="bg-input/50 h-9">
            <ListboxFilter v-model="query" auto-focus data-slot="command-input" class="w-full text-sm outline-hidden" :placeholder="`Search ${targetLabel}…`" :aria-label="`Search ${targetLabel}`" />
            <InputGroupAddon><SearchIcon class="size-4 shrink-0 opacity-50" aria-hidden="true" /></InputGroupAddon>
          </InputGroup>
        </div>
        <CommandList>
          <CommandGroup>
          <CommandItem v-if="!required && modelValue" value="__clear" @select="choose(null)"><XIcon aria-hidden="true" />Clear selection</CommandItem>
          <CommandItem v-for="item in items" :key="item.id" :value="item.id" @select="choose(item)">
            <span class="picker-label">{{ item.label }}</span>
            <CheckIcon v-if="item.id === modelValue" class="picker-check" aria-hidden="true" />
          </CommandItem>
          <p v-if="loading" class="picker-note" role="status">Loading…</p>
          <p v-else-if="items.length === 0" class="picker-note" role="status">{{ query ? `No ${targetLabel} records match.` : `No ${targetLabel} records yet.` }}</p>
          <CommandItem v-if="nextPageToken && !loading" value="__more" @select.prevent="search(true)">Show more</CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    </PopoverContent>
  </Popover>
</template>
