// ErrorState covers a page that failed to load, with a retry.
<script setup lang="ts">
import { CircleAlert } from '@lucide/vue'
import { Button } from '@/components/ui/button'

withDefaults(defineProps<{ title?: string; message?: string }>(), {
  title: 'This page did not load',
  message: 'Check your connection and try again.',
})
const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="flex flex-col items-center px-6 py-12 text-center">
    <div class="bg-muted flex size-10 items-center justify-center rounded-lg border">
      <CircleAlert class="text-muted-foreground size-5" aria-hidden="true" />
    </div>
    <h2 class="mt-4 text-base font-medium">{{ title }}</h2>
    <p class="text-muted-foreground mt-1 max-w-sm text-sm">{{ message }}</p>
    <div class="mt-4">
      <Button variant="outline" @click="emit('retry')">Try again</Button>
    </div>
  </div>
</template>
