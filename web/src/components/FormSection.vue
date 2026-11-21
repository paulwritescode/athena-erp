// FormSection groups one idea of a form or page inside a card: a title, one
// line saying what lives here, and the content.
<script setup lang="ts">
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'

defineProps<{ title: string; description?: string }>()
</script>

<template>
  <Card>
    <CardHeader>
      <h2>{{ title }}</h2>
      <CardDescription v-if="description" class="text-caption">{{ description }}</CardDescription>
    </CardHeader>
    <CardContent>
      <slot />
    </CardContent>
  </Card>
</template>
