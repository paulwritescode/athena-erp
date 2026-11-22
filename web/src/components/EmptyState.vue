// EmptyState is the shared "nothing here yet" block: a hairline figure from
// the art registry when the art key names one, else an icon in a bordered
// box; then a title, one helpful line, and an optional action.
<script setup lang="ts">
import { computed } from 'vue'
import type { Component } from 'vue'
import ArtFigure from '@/art/ArtFigure.vue'
import { useArt } from '@/art/registry'

const props = defineProps<{ icon?: Component; title: string; body: string; art?: string }>()

const figureKey = computed(() => {
  if (!props.art) return undefined
  return useArt(props.art)?.kind === 'figure' ? props.art : undefined
})
</script>

<template>
  <div class="flex flex-col items-center px-6 py-12 text-center">
    <ArtFigure v-if="figureKey" :art-key="figureKey" />
    <div v-else-if="icon" class="bg-muted flex size-10 items-center justify-center rounded-lg border">
      <component :is="icon" class="text-muted-foreground size-5" aria-hidden="true" />
    </div>
    <h2 :class="figureKey ? 'mt-8' : 'mt-4'">{{ title }}</h2>
    <p class="text-muted-foreground mt-1 max-w-sm text-base">{{ body }}</p>
    <div v-if="$slots.action" class="mt-4">
      <slot name="action" />
    </div>
  </div>
</template>
