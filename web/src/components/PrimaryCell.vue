// PrimaryCell is the table's primary column (§21): the thing's name with an
// optional 32 px icon tile and a muted second line, as in a transaction
// list. Callers render it inside their primary column's cell. Ledger codes
// render mono.
<script setup lang="ts">
import { icon, type IconName } from '@/icons'
import type { Component } from 'vue'
import { computed } from 'vue'

const props = defineProps<{
  title: string
  subtitle?: string | null
  icon?: IconName | Component | null
  mono?: boolean
}>()

const tileIcon = computed(() => {
  if (!props.icon) return null
  return typeof props.icon === 'string' ? icon(props.icon) : props.icon
})
</script>

<template>
  <span class="flex min-w-0 items-center gap-3">
    <span v-if="tileIcon" class="bg-muted grid size-8 shrink-0 place-items-center rounded-[calc(var(--radius)-2px)] border" aria-hidden="true">
      <component :is="tileIcon" class="text-muted-foreground size-4" />
    </span>
    <span class="min-w-0">
      <span class="block truncate font-medium" :class="mono ? 'font-mono' : undefined">{{ title }}</span>
      <span v-if="subtitle" class="text-muted-foreground block truncate text-caption">{{ subtitle }}</span>
    </span>
  </span>
</template>
