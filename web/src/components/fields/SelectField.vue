// SelectField is the shared §7 single-choice input: a searchable combobox
// on desktop, a native select below 1024 px (floating layers cannot nest
// inside drawers).
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Check, ChevronsUpDown } from '@/icons'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import FieldShell from './FieldShell.vue'

export type SelectOption = { value: string; label: string }

const props = defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  options: SelectOption[]
  modelValue?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; blur: [] }>()

const isDesktop = useMediaQuery('(min-width: 1024px)')
const open = ref(false)

const chosen = computed(() => props.options.find((option) => option.value === props.modelValue) ?? null)

function choose(value: string) {
  emit('update:modelValue', value)
  open.value = false
}
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <NativeSelect
        v-if="!isDesktop"
        :id="id"
        :model-value="modelValue ?? ''"
        :disabled="disabled"
        :aria-invalid="invalid"
        :aria-required="!optional || undefined"
        :aria-describedby="describedby"
        :aria-label="label"
        class="h-11"
        @update:model-value="$emit('update:modelValue', String($event))"
        @blur="$emit('blur')"
      >
        <NativeSelectOption value="">{{ placeholder ?? 'Choose…' }}</NativeSelectOption>
        <NativeSelectOption v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</NativeSelectOption>
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
            @blur="$emit('blur')"
          >
            <span :class="chosen ? undefined : 'muted-label'">{{ chosen?.label ?? placeholder ?? 'Choose…' }}</span>
            <ChevronsUpDown class="size-4 shrink-0 opacity-50" aria-hidden="true" />
          </Button>
        </PopoverTrigger>
        <PopoverContent class="w-(--reka-popover-trigger-width) p-0" align="start">
          <Command>
            <CommandInput :placeholder="`Search ${label.toLowerCase()}…`" />
            <CommandList>
              <CommandEmpty>Nothing matches.</CommandEmpty>
              <CommandGroup>
                <CommandItem v-if="optional" key="__none__" value="" @select="choose('')">
                  <Check :class="['size-4', !modelValue ? 'opacity-100' : 'opacity-0']" aria-hidden="true" />
                  <span>None</span>
                </CommandItem>
                <CommandItem v-for="option in options" :key="option.value" :value="option.value" @select="choose(option.value)">
                  <Check :class="['size-4', option.value === modelValue ? 'opacity-100' : 'opacity-0']" aria-hidden="true" />
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
