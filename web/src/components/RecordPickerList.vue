// The RecordPicker search list, shared by its popover (tablet and desktop)
// and its bottom drawer (phones and tablets).
<script setup lang="ts">
import { Check, Search, X } from '@/icons'
import { Command, CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { InputGroup, InputGroupAddon } from '@/components/ui/input-group'
import { ListboxFilter } from 'reka-ui'
import type { LookupItem } from '../gen/athena/v1/records_pb'

defineProps<{
  targetLabel: string
  items: LookupItem[]
  loading: boolean
  nextPageToken: string
  modelValue: string
  canClear: boolean
  autoFocus?: boolean
}>()
const emit = defineEmits<{ 'update:query': [value: string]; select: [item: LookupItem | null]; more: [] }>()

const query = defineModel<string>('query', { required: true })
</script>

<template>
  <Command>
    <!-- The server filters, so the search text stays out of the Command's own client-side filter. -->
    <div class="p-1 pb-0">
      <InputGroup class="bg-input/50 h-9">
        <ListboxFilter
          v-model="query"
          :auto-focus="autoFocus ?? true"
          data-slot="command-input"
          class="w-full text-sm outline-hidden"
          :placeholder="`Search ${targetLabel}…`"
          :aria-label="`Search ${targetLabel}`"
        />
        <InputGroupAddon><Search class="size-4 shrink-0 opacity-50" aria-hidden="true" /></InputGroupAddon>
      </InputGroup>
    </div>
    <CommandList>
      <CommandGroup>
        <CommandItem v-if="canClear" value="__clear" @select="emit('select', null)"><X aria-hidden="true" />Clear selection</CommandItem>
        <CommandItem v-for="item in items" :key="item.id" :value="item.id" @select="emit('select', item)">
          <span class="picker-label">{{ item.label }}</span>
          <Check v-if="item.id === modelValue" class="picker-check" aria-hidden="true" />
        </CommandItem>
        <p v-if="loading" class="picker-note" role="status">Loading…</p>
        <p v-else-if="items.length === 0" class="picker-note" role="status">{{ query ? `No ${targetLabel} records match.` : `No ${targetLabel} records yet.` }}</p>
        <CommandItem v-if="nextPageToken && !loading" value="__more" @select.prevent="emit('more')">Show more</CommandItem>
      </CommandGroup>
    </CommandList>
  </Command>
</template>
