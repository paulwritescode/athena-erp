// AppSidebar is desk navigation at 640 px and up: full sidebar at >=1024,
// icon rail from 640 to 1023 (the Sidebar collapses itself from the open
// state App.vue drives). Build lists registry pages; View lists saved Record
// Types, then the Work queue and Count.
<script setup lang="ts">
import { computed, nextTick, useTemplateRef, watch } from 'vue'
import { onKeyStroke } from '@vueuse/core'
import { FocusScope } from 'reka-ui'
import markDark from '@/assets/brand/athena-white.png'
import markLight from '@/assets/brand/athena-black.png'
import { PanelLeftClose, PanelLeftOpen } from '@lucide/vue'
import { Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuBadge, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import { recordTypeIcon, visibleBuildEntries, visibleViewStubs, type PageEntry } from '@/pages/registry'
import type { Page, RecordType } from '@/types'

const props = defineProps<{
  expanded: boolean
  // Tablet with the rail expanded: the sidebar overlays content instead of
  // pushing it, with a dim layer, a focus trap and Esc/outside/nav dismissal.
  overlay: boolean
  mode: 'build' | 'view'
  activePage: Page
  canBuild: boolean
  permissions: string[]
  recordTypes: RecordType[]
  viewRecordTypes: RecordType[]
  // The selected Record Type while View shows Records, else null.
  activeRecordType: string | null
  workspaceName: string
}>()
const emit = defineEmits<{ home: []; navigate: [page: Page]; recordType: [name: string]; toggle: [] }>()

const buildEntries = computed(() => visibleBuildEntries(props.canBuild, props.permissions))
const viewStubs = computed(() => visibleViewStubs())

const sidebarRef = useTemplateRef('sidebarRef')
watch(() => props.overlay, (open) => {
  if (!open) return
  void nextTick(() => {
    const root = sidebarRef.value?.$el as HTMLElement | undefined
    root?.querySelector('button')?.focus()
  })
})

onKeyStroke('Escape', () => {
  if (props.overlay) emit('toggle')
})

function goHome() {
  if (props.overlay) emit('toggle')
  emit('home')
}

function go(page: Page) {
  if (props.overlay) emit('toggle')
  emit('navigate', page)
}

function goRecordType(name: string) {
  if (props.overlay) emit('toggle')
  emit('recordType', name)
}

function badge(entry: PageEntry): string | null {
  return entry.key === 'record-types' ? String(props.recordTypes.length) : null
}
</script>

<template>
  <div v-if="overlay" class="fixed inset-0 z-40 bg-black/30" aria-hidden="true" @click="emit('toggle')" />
  <FocusScope :trapped="overlay" :loop="overlay" as-child>
    <Sidebar ref="sidebarRef" collapsible="icon" :class="overlay ? 'z-50' : undefined">
      <SidebarHeader>
        <button class="flex w-full items-center gap-2 rounded-md p-2 text-left" aria-label="Athena home" @click="goHome()">
        <img :src="markLight" alt="" class="size-8 shrink-0 rounded-md dark:hidden" />
        <img :src="markDark" alt="" class="hidden size-8 shrink-0 rounded-md dark:block" />
        <span class="min-w-0 group-data-[collapsible=icon]:hidden">
          <span class="block text-sm leading-tight font-semibold">Athena</span>
          <span v-if="workspaceName" class="text-muted-foreground block truncate text-xs leading-tight">{{ workspaceName }}</span>
        </span>
      </button>
    </SidebarHeader>
    <SidebarContent>
      <SidebarGroup v-if="mode === 'build'">
        <SidebarGroupLabel>Build</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem v-for="entry in buildEntries" :key="entry.key">
              <SidebarMenuButton :is-active="activePage === entry.key" :tooltip="entry.title" @click="go(entry.key)">
                <component :is="entry.icon" aria-hidden="true" />
                <span>{{ entry.title }}</span>
                <SidebarMenuBadge v-if="badge(entry)">{{ badge(entry) }}</SidebarMenuBadge>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
      <template v-else>
        <SidebarGroup>
          <SidebarGroupLabel>Records</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu v-if="viewRecordTypes.length > 0">
              <SidebarMenuItem v-for="recordType in viewRecordTypes" :key="recordType.name">
                <SidebarMenuButton :is-active="recordType.name === activeRecordType" :tooltip="recordType.label" @click="goRecordType(recordType.name)">
                  <component :is="recordTypeIcon" aria-hidden="true" />
                  <span>{{ recordType.label }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
            <p v-else class="text-muted-foreground px-2 py-1 text-xs">No saved Record Types.</p>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem v-for="entry in viewStubs" :key="entry.key">
                <SidebarMenuButton :is-active="activePage === entry.key" :tooltip="entry.title" @click="go(entry.key)">
                  <component :is="entry.icon" aria-hidden="true" />
                  <span>{{ entry.title }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </template>
    </SidebarContent>
    <SidebarFooter>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            :tooltip="expanded ? 'Collapse sidebar' : 'Expand sidebar'"
            :aria-label="expanded ? 'Collapse sidebar' : 'Expand sidebar'"
            @click="emit('toggle')"
          >
            <component :is="expanded ? PanelLeftClose : PanelLeftOpen" aria-hidden="true" />
            <span>{{ expanded ? 'Collapse' : 'Expand' }}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarFooter>
    </Sidebar>
  </FocusScope>
</template>
