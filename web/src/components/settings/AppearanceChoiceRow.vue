// One Appearance choice row (§18): a caption label, the current value in
// body text and an indicator on the right. Clicking opens a popover on
// desktop, a bottom drawer on touch or narrow screens.
<script setup lang="ts">
import { ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { ChevronRight } from '@/icons'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

withDefaults(defineProps<{ label: string; value: string; disabled?: boolean; drawerTitle: string; popoverClass?: string }>(), {
  disabled: false,
  popoverClass: 'w-72',
})

const open = ref(false)
const useDrawer = useMediaQuery('(pointer: coarse), (max-width: 639px)')

function close() {
  open.value = false
}
</script>

<template>
  <Popover v-if="!useDrawer" v-model:open="open">
    <PopoverTrigger as-child :disabled="disabled">
      <button
        type="button"
        :disabled="disabled"
        class="flex w-full items-center gap-3 rounded-[var(--radius)] border border-border bg-card px-4 py-3 text-left transition-colors hover:bg-muted/50 disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-card"
      >
        <span class="min-w-0 flex-1">
          <span class="muted-label block">{{ label }}</span>
          <span class="block truncate text-sm font-medium text-foreground">{{ value }}</span>
        </span>
        <slot name="indicator" />
        <ChevronRight class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      </button>
    </PopoverTrigger>
    <PopoverContent align="start" :class="popoverClass">
      <slot name="options" :close="close" />
    </PopoverContent>
  </Popover>
  <template v-else>
    <button
      type="button"
      :disabled="disabled"
      class="flex w-full items-center gap-3 rounded-[var(--radius)] border border-border bg-card px-4 py-3 text-left transition-colors hover:bg-muted/50 disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-card"
      @click="open = true"
    >
      <span class="min-w-0 flex-1">
        <span class="muted-label block">{{ label }}</span>
        <span class="block truncate text-sm font-medium text-foreground">{{ value }}</span>
      </span>
      <slot name="indicator" />
      <ChevronRight class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
    <BottomDrawer v-model:open="open" :title="drawerTitle">
      <slot name="options" :close="close" />
    </BottomDrawer>
  </template>
</template>
