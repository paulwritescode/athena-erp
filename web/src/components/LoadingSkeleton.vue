// LoadingSkeleton holds the page shape while its code loads.
<script setup lang="ts">
import { Skeleton } from '@/components/ui/skeleton'

withDefaults(defineProps<{ rows?: number }>(), { rows: 3 })
</script>

<template>
  <div class="space-y-3" aria-busy="true" aria-label="Loading">
    <Skeleton class="h-8 w-48 max-w-full" />
    <Skeleton class="h-4 w-72 max-w-full" />
    <Skeleton v-for="row in rows" :key="row" class="h-16 w-full" />
  </div>
</template>
