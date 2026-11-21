// MultiSelectField is the shared §7 multi-choice input: chosen options show
// as removable chips above a combobox on desktop, or a native add-one select
// below 1024 px. The model is the array of chosen values.
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Check, ChevronsUpDown, X } from '@/icons'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import FieldShell from './FieldShell.vue'
import type { SelectOption } from './SelectField.vue'

const props = defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  options: SelectOption[]
  modelValue?: string[]
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

const isDesktop = useMediaQuery('(min-width: 1024px)')
const open = ref(false)
const chosen = computed(() => props.modelValue ?? [])

function toggle(value: string) {
  const next = chosen.value.includes(value) ? chosen.value.filter((item) => item !== value) : [...chosen.value, value]
  emit('update:modelValue', next)
}

function labelFor(value: string): string {
  return props.options.find((option) => option.value === value)?.label ?? value
}

function addFromNative(value: string) {
  if (value && !chosen.value.includes(value)) emit('update:modelValue', [...chosen.value, value])
}
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <div v-if="chosen.length > 0" class="flex flex-wrap gap-1.5" role="list" :aria-label="`Chosen ${label.toLowerCase()}`">
        <Badge v-for="value in chosen" :key="value" variant="secondary" role="listitem" class="gap-1">
          <span>{{ labelFor(value) }}</span>
          <button
            type="button"
            :aria-label="`Remove ${labelFor(value)}`"
            :disabled="disabled"
            class="-mr-1 rounded-full p-0.5 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none"
            @click="toggle(value)"
          >
            <X class="size-3" aria-hidden="true" />
          </button>
        </Badge>
      </div>
      <NativeSelect
        v-if="!isDesktop"
        :id="id"
        model-value=""
        :disabled="disabled"
        :aria-invalid="invalid"
        :aria-required="!optional || undefined"
        :aria-describedby="describedby"
        :aria-label="`${label}: add one`"
        class="h-11"
        @update:model-value="addFromNative(String($event))"
      >
        <NativeSelectOption value="">{{ placeholder ?? 'Add one…' }}</NativeSelectOption>
        <NativeSelectOption v-for="option in options.filter((item) => !chosen.includes(item.value))" :key="option.value" :value="option.value">
          {{ option.label }}
        </NativeSelectOption>
      </NativeSelect>
      <Popover v-else v-model:open="open">
        <PopoverTrigger as-child>
          <Button
            :id="id"
            type="button"
            variant="outline"
            role="combobox"
            :aria-expanded="open"
            :disabled="disabled"
            :aria-invalid="invalid"
            :aria-required="!optional || undefined"
            :aria-describedby="describedby"
            :aria-label="label"
            class="h-11 w-full justify-between font-normal min-[640px]:h-10 lg:h-9"
          >
            <span class="muted-label">{{ chosen.length > 0 ? `${chosen.length} selected` : (placeholder ?? 'Choose…') }}</span>
            <ChevronsUpDown class="size-4 shrink-0 opacity-50" aria-hidden="true" />
          </Button>
        </PopoverTrigger>
        <PopoverContent class="w-(--reka-popover-trigger-width) p-0" align="start">
          <Command>
            <CommandInput :placeholder="`Search ${label.toLowerCase()}…`" />
            <CommandList>
              <CommandEmpty>Nothing matches.</CommandEmpty>
              <CommandGroup>
                <CommandItem v-for="option in options" :key="option.value" :value="option.value" @select="toggle(option.value)">
                  <Check :class="['size-4', chosen.includes(option.value) ? 'opacity-100' : 'opacity-0']" aria-hidden="true" />
                  <span>{{ option.label }}</span>
                </CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </template>
  </FieldShell>
</template>
