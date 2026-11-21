// BottomNav is phone navigation below 640 px: the first four destinations
// plus a More sheet with the rest. Build shows registry pages; View shows
// saved Record Types, then the Work queue and Count.
<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { Ellipsis } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { recordTypeIcon, visibleViewStubs, type PageEntry } from '@/pages/registry'
import type { Page, RecordType } from '@/types'

type NavItem = { key: string; label: string; icon: Component; active: boolean; go: () => void }

const props = defineProps<{
  mode: 'build' | 'view'
  buildEntries: PageEntry[]
  viewRecordTypes: RecordType[]
  activePage: Page
  // The selected Record Type while View shows Records, else null.
  activeRecordType: string | null
}>()
const emit = defineEmits<{ navigate: [page: Page]; recordType: [name: string] }>()

const moreOpen = ref(false)

const items = computed<NavItem[]>(() => {
  if (props.mode === 'build') {
    return props.buildEntries.map((entry) => ({
      key: entry.key,
      label: entry.shortTitle,
      icon: entry.icon,
      active: props.activePage === entry.key,
      go: () => emit('navigate', entry.key),
    }))
  }
  const types: NavItem[] = props.viewRecordTypes.map((recordType) => ({
    key: `record:${recordType.name}`,
    label: recordType.label,
    icon: recordTypeIcon,
    active: recordType.name === props.activeRecordType,
    go: () => emit('recordType', recordType.name),
  }))
  const stubs: NavItem[] = visibleViewStubs().map((entry) => ({
    key: entry.key,
    label: entry.shortTitle,
    icon: entry.icon,
    active: props.activePage === entry.key,
    go: () => emit('navigate', entry.key),
  }))
  return [...types, ...stubs]
})

const primary = computed(() => items.value.slice(0, 4))
const overflow = computed(() => items.value.slice(4))
const overflowActive = computed(() => overflow.value.some((item) => item.active))

function go(item: NavItem) {
  moreOpen.value = false
  item.go()
}
</script>

<template>
  <nav class="bg-background/95 supports-[backdrop-filter]:bg-background/60 fixed inset-x-0 bottom-0 z-20 border-t backdrop-blur min-[640px]:hidden" aria-label="Primary">
    <div class="flex items-start px-2 pt-1 pb-[calc(0.25rem+env(safe-area-inset-bottom))]">
      <button
        v-for="item in primary"
        :key="item.key"
        class="flex min-w-0 flex-1 flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] leading-tight"
        :class="item.active ? 'text-primary font-medium' : 'text-muted-foreground'"
        :aria-current="item.active ? 'page' : undefined"
        @click="item.go()"
      >
        <component :is="item.icon" class="size-5" aria-hidden="true" />
        <span class="w-full text-center leading-tight break-words">{{ item.label }}</span>
      </button>
      <button
        v-if="overflow.length > 0"
        class="flex min-w-0 flex-1 flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] leading-tight"
        :class="overflowActive ? 'text-primary font-medium' : 'text-muted-foreground'"
        :aria-expanded="moreOpen"
        @click="moreOpen = true"
      >
        <Ellipsis class="size-5" aria-hidden="true" />
        <span class="w-full text-center leading-tight">More</span>
      </button>
    </div>
  </nav>
  <Sheet v-model:open="moreOpen">
    <SheetContent side="bottom" class="px-4" aria-label="More pages">
      <SheetHeader class="px-0 text-left">
        <SheetTitle>More</SheetTitle>
      </SheetHeader>
      <div class="flex flex-col gap-1 pb-4">
        <Button
          v-for="item in overflow"
          :key="item.key"
          variant="ghost"
          class="justify-start"
          :class="item.active ? 'text-primary' : undefined"
          @click="go(item)"
        >
          <component :is="item.icon" class="size-4" aria-hidden="true" />
          <span>{{ item.label }}</span>
        </Button>
      </div>
    </SheetContent>
  </Sheet>
</template>
