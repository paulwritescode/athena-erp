// FormSection groups one idea of a form or page inside a card: a title, one
// line saying what lives here, and the content.
<script setup lang="ts">
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

defineProps<{ title: string; description?: string }>()
</script>

<template>
  <Card class="rounded-[var(--radius)] shadow-none">
    <CardHeader>
      <CardTitle>{{ title }}</CardTitle>
      <CardDescription v-if="description">{{ description }}</CardDescription>
    </CardHeader>
    <CardContent>
      <slot />
    </CardContent>
  </Card>
</template>
