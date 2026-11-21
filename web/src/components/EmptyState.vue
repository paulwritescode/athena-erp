// EmptyState is the shared "nothing here yet" block: an icon in a bordered
// box, a title, one helpful line, and an optional action.
<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{ icon: Component; title: string; body: string }>()
</script>

<template>
  <div class="flex flex-col items-center px-6 py-12 text-center">
    <div class="bg-muted flex size-10 items-center justify-center rounded-lg border">
      <component :is="icon" class="text-muted-foreground size-5" aria-hidden="true" />
    </div>
    <h2 class="mt-4">{{ title }}</h2>
    <p class="text-muted-foreground mt-1 max-w-sm text-base">{{ body }}</p>
    <div v-if="$slots.action" class="mt-4">
      <slot name="action" />
    </div>
  </div>
</template>
