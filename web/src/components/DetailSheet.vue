// DetailSheet shows one object in a right sheet that spans the full viewport
// height, per design-system section 16: its own header with title, status and
// close, a scrolling body, a sticky footer for the section 14 action groups,
// and a dim overlay. It is 480 px wide, 640 px with wide (detail over form);
// on phones it is a vaul bottom drawer with swipe to close.
<script setup lang="ts">
import { onUnmounted, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import BottomDrawer from './BottomDrawer.vue'
import { claimOverlayTier, registerSheet } from './overlayPresence'

const props = defineProps<{ title: string; description?: string; wide?: boolean }>()
const open = defineModel<boolean>('open', { required: true })
// Phones get a bottom sheet; larger screens keep the right-side sheet.
const isPhone = useMediaQuery('(max-width: 639px)')
// While open, toasts move beside the sheet (§17); the media query owns phones.
let releaseToasts: (() => void) | undefined
watch(open, (isOpen) => {
  releaseToasts?.()
  releaseToasts = undefined
  if (isOpen) releaseToasts = registerSheet(props.wide ?? false)
}, { immediate: true })
// The right-sheet half claims a stacking tier; the phone half is a
// BottomDrawer and claims itself. Watching the breakpoint too keeps the
// count right when a resize swaps halves mid-open.
let releaseTier: (() => void) | undefined
watch([open, isPhone], ([isOpen, phone]) => {
  releaseTier?.()
  releaseTier = undefined
  if (isOpen && !phone) releaseTier = claimOverlayTier().release
}, { immediate: true })
onUnmounted(() => {
  releaseToasts?.()
  releaseToasts = undefined
  releaseTier?.()
  releaseTier = undefined
})
</script>

<template>
  <Sheet v-if="!isPhone" v-model:open="open">
    <SheetContent
      side="right"
      :class="[
        'px-4',
        'sm:max-w-[calc(100vw-2rem)]',
        // Same variant stack as the sheet base
        // (data-[side=right]:w-3/4, data-[side=right]:sm:max-w-sm) so the
        // merge keeps these widths.
        wide
          ? 'data-[side=right]:w-[640px] data-[side=right]:sm:max-w-[640px]'
          : 'data-[side=right]:w-[480px] data-[side=right]:sm:max-w-[480px]',
      ]"
    >
      <SheetHeader class="px-0 pr-10 text-left">
        <div class="flex flex-wrap items-center gap-2">
          <SheetTitle>{{ title }}</SheetTitle>
          <slot name="status" />
        </div>
        <slot name="subtitle" />
        <SheetDescription v-if="description">{{ description }}</SheetDescription>
      </SheetHeader>
      <div class="overlay-stack min-h-0 flex-1 overflow-y-auto py-4">
        <slot />
      </div>
      <SheetFooter v-if="$slots.footer" class="px-0">
        <slot name="footer" />
      </SheetFooter>
    </SheetContent>
  </Sheet>
  <BottomDrawer v-else v-model:open="open" :title="title" :description="description">
    <template #status>
      <slot name="status" />
    </template>
    <template #subtitle>
      <slot name="subtitle" />
    </template>
    <slot />
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </BottomDrawer>
</template>
