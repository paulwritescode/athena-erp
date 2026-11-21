// BottomDrawer is the one vaul bottom drawer (§13.2): content height up to
// 92 dvh with the body scrolling inside, a grab handle, close on a downward
// swipe past 30% or a fast flick, a plain dim overlay, a 44 px close target,
// and a footer pinned above the safe area. The surface spans the viewport;
// from 640 px the content caps at 640 and centers. ResponsiveDialog and the
// phone DetailSheet render it. A dirty drawer raises its close threshold to
// full height so exploratory drags snap back; vaul still reports real
// dismissals through update:open, which the caller may guard.
<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { DrawerClose, DrawerContent, DrawerDescription, DrawerOverlay, DrawerPortal, DrawerRoot, DrawerTitle } from 'vaul-vue'
import { X } from '@/icons'
import { claimOverlayTier, registerDrawer } from './overlayPresence'

const props = withDefaults(defineProps<{
  title: string
  description?: string
  dismissible?: boolean
  dirty?: boolean
}>(), { dismissible: true, dirty: false })
const open = defineModel<boolean>('open', { required: true })
const child = ref(false)

let release: (() => void) | undefined
let releaseTier: (() => void) | undefined
watch(open, (isOpen) => {
  release?.()
  release = undefined
  releaseTier?.()
  releaseTier = undefined
  if (isOpen) {
    release = registerDrawer()
    const tier = claimOverlayTier()
    child.value = tier.child
    releaseTier = tier.release
  }
}, { immediate: true })
onUnmounted(() => {
  release?.()
  release = undefined
  releaseTier?.()
  releaseTier = undefined
})
</script>

<template>
  <DrawerRoot :open="open" :dismissible="dismissible" :close-threshold="dirty ? 1 : 0.3" :modal="true" @update:open="$emit('update:open', $event)">
    <DrawerPortal>
      <DrawerOverlay :class="['bg-black/30 fixed inset-0 z-50', child ? 'overlay-child' : undefined]" />
      <DrawerContent
        :class="[
          'bg-popover text-popover-foreground fixed inset-x-0 bottom-0 z-50 flex max-h-[92dvh] w-full flex-col rounded-t-[var(--radius)] px-4 pt-3 pb-[env(safe-area-inset-bottom,0px)] outline-none',
          child ? 'overlay-child' : undefined,
        ]"
      >
        <div class="mx-auto flex min-h-0 w-full min-w-0 flex-1 flex-col min-[640px]:max-w-[640px]">
          <div aria-hidden="true" class="bg-muted-foreground/30 mx-auto mb-2 h-1 w-10 shrink-0 rounded-full" />
          <div class="flex items-start justify-between gap-2 pr-14">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <DrawerTitle class="text-foreground cn-font-heading text-base font-medium">{{ title }}</DrawerTitle>
                <slot name="status" />
              </div>
              <slot name="subtitle" />
              <DrawerDescription v-if="description" class="text-muted-foreground mt-1 text-sm">{{ description }}</DrawerDescription>
            </div>
          </div>
          <div v-if="$slots.default" class="overlay-stack min-h-0 flex-1 overflow-y-auto py-4">
            <slot />
          </div>
          <div v-if="$slots.footer" class="drawer-footer shrink-0 px-0 pt-2 pb-4">
            <slot name="footer" />
          </div>
        </div>
        <DrawerClose
          v-if="dismissible"
          aria-label="Close"
          class="text-muted-foreground hover:text-foreground hover:bg-muted focus-visible:ring-ring absolute top-3 right-3 grid size-11 shrink-0 place-items-center rounded-full outline-hidden focus-visible:ring-2"
        >
          <X class="size-4" aria-hidden="true" />
        </DrawerClose>
      </DrawerContent>
    </DrawerPortal>
  </DrawerRoot>
</template>
