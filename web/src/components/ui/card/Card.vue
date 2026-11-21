<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from 'cn'

const props = withDefaults(defineProps<{
  class?: HTMLAttributes['class']
  size?: 'default' | 'sm'
}>(), {
  size: 'default',
})
</script>

<template>
  <div
    data-slot="card"
    :data-size="size"
    :class="cn('bg-card text-card-foreground ring-foreground/5 dark:ring-foreground/10 gap-6 overflow-hidden rounded-lg py-6 text-sm ring-1 has-[>img:first-child]:pt-0 data-[size=sm]:gap-4 data-[size=sm]:py-4 *:[img:first-child]:rounded-t-lg *:[img:last-child]:rounded-b-lg group/card flex flex-col', props.class)"
  >
    <slot />
  </div>
</template>
