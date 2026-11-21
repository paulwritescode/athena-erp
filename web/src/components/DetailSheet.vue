// DetailSheet shows one object beside the page: a title, what it is, the
// content, and optional footer actions. It slides in from the right and
// scrolls when the content outgrows the viewport.
<script setup lang="ts">
import { useMediaQuery } from '@vueuse/core'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'

defineProps<{ title: string; description?: string }>()
const open = defineModel<boolean>('open', { required: true })
// Phones get a bottom sheet; larger screens keep the right-side sheet.
const isPhone = useMediaQuery('(max-width: 639px)')
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent :side="isPhone ? 'bottom' : 'right'" class="px-4">
      <SheetHeader class="px-0 text-left">
        <SheetTitle>{{ title }}</SheetTitle>
        <SheetDescription v-if="description">{{ description }}</SheetDescription>
      </SheetHeader>
      <div class="min-h-0 flex-1 overflow-y-auto py-4">
        <slot />
      </div>
      <SheetFooter v-if="$slots.footer" class="px-0">
        <slot name="footer" />
      </SheetFooter>
    </SheetContent>
  </Sheet>
</template>
