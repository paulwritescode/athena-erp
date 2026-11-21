// DateField is the shared §7 date input: a Calendar popover on desktop, a
// native date input below 1024 px where the OS picker is better and floating
// layers cannot nest inside drawers. The model is a YYYY-MM-DD string.
<script setup lang="ts">
import { computed } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { parseDate } from '@internationalized/date'
import type { DateValue } from 'reka-ui'
import { CalendarDays } from '@/icons'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import FieldShell from './FieldShell.vue'

const props = defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  placeholder?: string
  modelValue?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; blur: [] }>()

const isDesktop = useMediaQuery('(min-width: 1024px)')

const picked = computed<DateValue | undefined>(() => {
  if (!props.modelValue) return undefined
  try {
    return parseDate(props.modelValue)
  } catch {
    return undefined
  }
})

function onPick(next: unknown) {
  const value = typeof (next as DateValue | undefined)?.toString === 'function' ? String(next).slice(0, 10) : ''
  emit('update:modelValue', value)
}

function shown(): string {
  if (!props.modelValue) return props.placeholder ?? 'Pick a date'
  const [year, month, day] = props.modelValue.split('-')
  return year && month && day ? `${Number(day)} ${new Date(Number(year), Number(month) - 1, 1).toLocaleDateString(undefined, { month: 'short' })} ${year}` : props.modelValue
}
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="error">
    <template #default="{ describedby, invalid }">
      <Input
        v-if="!isDesktop"
        :id="id"
        :model-value="modelValue ?? ''"
        type="date"
        :disabled="disabled"
        :aria-invalid="invalid"
        :aria-required="!optional || undefined"
        :aria-describedby="describedby"
        class="h-11 min-[640px]:h-10 lg:h-9"
        @update:model-value="$emit('update:modelValue', String($event))"
        @blur="$emit('blur')"
      />
      <Popover v-else>
        <PopoverTrigger as-child>
          <Button
            :id="id"
            type="button"
            variant="outline"
            :disabled="disabled"
            :aria-invalid="invalid"
            :aria-required="!optional || undefined"
            :aria-describedby="describedby"
            :aria-label="label"
            class="h-11 w-full justify-start font-normal min-[640px]:h-10 lg:h-9"
            @blur="$emit('blur')"
          >
            <CalendarDays class="size-4 shrink-0 opacity-50" aria-hidden="true" />
            <span :class="modelValue ? undefined : 'muted-label'">{{ shown() }}</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent class="w-auto p-0" align="start">
          <Calendar :model-value="picked" @update:model-value="onPick" />
        </PopoverContent>
      </Popover>
    </template>
  </FieldShell>
</template>
