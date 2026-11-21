// PageHeader opens every desk page: a title, one line saying what the page
// is for, and optional actions on the right.
<script setup lang="ts">
defineProps<{ title: string; description?: string }>()
</script>

<template>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="min-w-0">
      <h1>{{ title }}</h1>
      <p v-if="description" class="text-muted-foreground text-caption mt-1">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>
