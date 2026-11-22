// CommandPalette jumps to pages and Records and runs desk actions. It opens
// from the header search field or Ctrl/⌘ K. A fuller action set (create a
// Record, recent items) lands with the safe row registry in B3.
<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { LogOut, Moon, Sun } from '@/icons'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from '@/components/ui/command'
import { recordTypeIcon, visibleViewStubs, type PageEntry } from '@/pages/registry'
import type { Page, RecordType } from '@/types'

const props = defineProps<{ buildEntries: PageEntry[]; recordTypes: RecordType[]; isDark: boolean; mode: 'build' | 'view' }>()
const emit = defineEmits<{ navigate: [page: Page]; recordType: [name: string]; toggleTheme: []; signOut: [] }>()
const open = defineModel<boolean>('open', { required: true })

const viewStubs = computed(() => visibleViewStubs())

function toggle(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    open.value = !open.value
  }
}

onMounted(() => window.addEventListener('keydown', toggle))
onUnmounted(() => window.removeEventListener('keydown', toggle))

function go(page: Page) {
  open.value = false
  emit('navigate', page)
}

function goRecordType(name: string) {
  open.value = false
  emit('recordType', name)
}
</script>

<template>
  <ResponsiveDialog v-model:open="open" title="Search" description="Pages, Records and actions.">
    <Command>
      <CommandInput placeholder="Search pages, Records and actions..." />
      <CommandList>
      <CommandEmpty>No matches.</CommandEmpty>
      <!-- View hides all Build navigation: the palette shows View lists
      and actions only. -->
      <CommandGroup v-if="mode === 'build'" heading="Build">
        <CommandItem v-for="entry in buildEntries" :key="entry.key" :value="`${entry.title} ${entry.blurb}`" @select="go(entry.key)">
          <component :is="entry.icon" class="size-4" aria-hidden="true" />
          <span>{{ entry.title }}</span>
        </CommandItem>
      </CommandGroup>
      <CommandGroup heading="View">
        <CommandItem v-for="entry in viewStubs" :key="entry.key" :value="`${entry.title} ${entry.blurb}`" @select="go(entry.key)">
          <component :is="entry.icon" class="size-4" aria-hidden="true" />
          <span>{{ entry.title }}</span>
        </CommandItem>
      </CommandGroup>
      <CommandGroup v-if="recordTypes.length > 0" heading="Records">
        <CommandItem v-for="recordType in recordTypes" :key="recordType.name" :value="`${recordType.label} ${recordType.name}`" @select="goRecordType(recordType.name)">
          <component :is="recordTypeIcon" class="size-4" aria-hidden="true" />
          <span>{{ recordType.label }}</span>
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="Actions">
        <CommandItem value="toggle theme dark light" @select="open = false; emit('toggleTheme')">
          <component :is="isDark ? Sun : Moon" class="size-4" aria-hidden="true" />
          <span>{{ isDark ? 'Light theme' : 'Dark theme' }}</span>
        </CommandItem>
        <CommandItem value="sign out log out" @select="open = false; emit('signOut')">
          <LogOut class="size-4" aria-hidden="true" />
          <span>Sign out</span>
        </CommandItem>
      </CommandGroup>
      </CommandList>
    </Command>
  </ResponsiveDialog>
</template>
