// BottomNav is floating phone navigation below 640 px (§13.1): inset from
// the sides and bottom, four destinations plus a More drawer. Build shows
// registry pages; View shows saved Record Types, then the Work queue and
// Count. It hides while the keyboard is open and while a drawer is open.
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, type Component } from 'vue'
import { Ellipsis } from '@/icons'
import BottomDrawer from '@/components/BottomDrawer.vue'
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
const keyboardOpen = ref(false)

function onViewportResize() {
  const viewport = window.visualViewport
  keyboardOpen.value = !!viewport && window.innerHeight - viewport.height > 150
}

onMounted(() => {
  window.visualViewport?.addEventListener('resize', onViewportResize)
  onViewportResize()
})
onUnmounted(() => {
  window.visualViewport?.removeEventListener('resize', onViewportResize)
})

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
const overflow = computed(() => items.value.length > 4)
const overflowItems = computed(() => items.value.slice(4))
const overflowActive = computed(() => overflowItems.value.some((item) => item.active))

function go(item: NavItem) {
  moreOpen.value = false
  item.go()
}
</script>

<template>
  <nav
    data-bottom-nav
    aria-label="Primary"
    :inert="keyboardOpen || undefined"
    :class="[
      'fixed inset-x-3 z-30 transition-[transform,opacity] duration-200 min-[640px]:hidden',
      keyboardOpen ? 'pointer-events-none translate-y-[130%] opacity-0' : undefined,
    ]"
    :style="{ bottom: 'calc(var(--space-3) + env(safe-area-inset-bottom, 0px))' }"
  >
    <div class="bg-card flex h-16 items-stretch gap-1 rounded-[calc(var(--radius)*2)] border px-2 py-1.5 shadow-xl">
      <button
        v-for="item in primary"
        :key="item.key"
        class="flex min-w-0 flex-1 flex-col items-center justify-center gap-0.5 rounded-xl px-1 text-caption leading-tight"
        :class="item.active ? 'text-primary font-medium' : 'text-muted-foreground'"
        :aria-current="item.active ? 'page' : undefined"
        @click="item.go()"
      >
        <component :is="item.icon" class="size-6" aria-hidden="true" />
        <span class="w-full text-center leading-tight break-words">{{ item.label }}</span>
      </button>
      <button
        v-if="overflow"
        class="flex min-w-0 flex-1 flex-col items-center justify-center gap-0.5 rounded-xl px-1 text-caption leading-tight"
        :class="overflowActive ? 'text-primary font-medium' : 'text-muted-foreground'"
        :aria-expanded="moreOpen"
        @click="moreOpen = true"
      >
        <Ellipsis class="size-6" aria-hidden="true" />
        <span class="w-full text-center leading-tight">More</span>
      </button>
    </div>
  </nav>
  <BottomDrawer v-model:open="moreOpen" title="More">
    <div class="drawer-menu" role="menu" aria-label="More pages">
      <button
        v-for="item in overflowItems"
        :key="item.key"
        type="button"
        role="menuitem"
        :aria-current="item.active ? 'page' : undefined"
        @click="go(item)"
      >
        <component :is="item.icon" class="size-4" aria-hidden="true" />
        <span>{{ item.label }}</span>
      </button>
    </div>
  </BottomDrawer>
</template>
