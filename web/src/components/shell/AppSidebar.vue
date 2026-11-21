// AppSidebar is desk navigation at 640 px and up: full sidebar at >=1024,
// icon rail from 640 to 1023 (the Sidebar collapses itself from the open
// state App.vue drives). Build lists registry pages in §15 groups; View
// lists Home, the saved Record Types grouped by Pack, then the stubs.
// Headed groups collapse; the open state is remembered per user.
<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { onKeyStroke } from '@vueuse/core'
import { FocusScope } from 'reka-ui'
import markDark from '@/assets/brand/athena-white.png'
import markLight from '@/assets/brand/athena-black.png'
import { ChevronDown, House, PanelLeftClose, PanelLeftOpen } from '@/icons'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarMenu, SidebarMenuBadge, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import { isEntryVisible, loadCollapsedGroups, pageEntry, recordTypeGroups, recordTypeIcon, saveCollapsedGroups, visibleBuildGroups, visibleViewStubs, type PageEntry } from '@/pages/registry'
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
  username: string
}>()
const emit = defineEmits<{ home: []; navigate: [page: Page]; recordType: [name: string]; toggle: [] }>()

const buildGroups = computed(() => visibleBuildGroups(props.canBuild, props.permissions))
const ungroupedBuild = computed(() => buildGroups.value.find((group) => group.key === null)?.entries ?? [])
const headedBuildGroups = computed(() =>
  buildGroups.value.filter((group): group is { key: string; label: string; entries: typeof group.entries } => group.key !== null),
)
const rtGroups = computed(() => recordTypeGroups(props.viewRecordTypes))
// View stubs in §15 order: Work queue, Reports when visible, Count.
const viewStubEntries = computed(() => {
  const stubs = [...visibleViewStubs()]
  const reports = pageEntry('reports')
  if (reports && isEntryVisible(reports, props.canBuild, props.permissions)) stubs.splice(1, 0, reports)
  return stubs
})

const collapsed = ref(loadCollapsedGroups(props.username))
watch(() => props.username, (name) => {
  collapsed.value = loadCollapsedGroups(name)
})

// The rail shows every icon; collapse only applies expanded.
function groupOpen(key: string): boolean {
  return !props.expanded || !collapsed.value.has(key)
}

function setGroupOpen(key: string, open: boolean) {
  const next = new Set(collapsed.value)
  if (open) next.delete(key)
  else next.add(key)
  collapsed.value = next
  saveCollapsedGroups(props.username, next)
}

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

// The scope autofocuses its first control on mount; the rail must never
// steal focus on load. Opening the overlay focuses explicitly instead.
function guardAutofocus(event: Event) {
  if (!props.overlay) event.preventDefault()
}

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
  <div v-if="overlay" class="fixed inset-0 z-20 bg-black/30" aria-hidden="true" @click="emit('toggle')" />
  <FocusScope :trapped="overlay" :loop="overlay" as-child @mount-auto-focus="guardAutofocus">
    <Sidebar ref="sidebarRef" collapsible="icon" :class="overlay ? 'z-30' : undefined">
      <SidebarHeader>
        <button class="flex w-full items-center gap-2 rounded-md p-2 pl-5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar" aria-label="Athena home" @click="goHome()">
        <img :src="markLight" alt="" class="size-8 shrink-0 rounded-md dark:hidden" />
        <img :src="markDark" alt="" class="hidden size-8 shrink-0 rounded-md dark:block" />
        <span class="min-w-0 group-data-[collapsible=icon]:hidden">
          <span class="block text-sm leading-tight font-semibold">Athena</span>
          <span v-if="workspaceName" class="text-muted-foreground block truncate text-xs leading-tight">{{ workspaceName }}</span>
        </span>
      </button>
    </SidebarHeader>
    <SidebarContent>
      <template v-if="mode === 'build'">
        <SidebarGroup v-if="ungroupedBuild.length > 0">
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem v-for="entry in ungroupedBuild" :key="entry.key">
                <SidebarMenuButton :is-active="activePage === entry.key" :tooltip="entry.title" @click="go(entry.key)">
                  <component :is="entry.icon" aria-hidden="true" />
                  <span>{{ entry.title }}</span>
                  <SidebarMenuBadge v-if="badge(entry)">{{ badge(entry) }}</SidebarMenuBadge>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup v-for="group in headedBuildGroups" :key="group.key">
          <Collapsible :open="groupOpen(group.key)" @update:open="(open: boolean) => setGroupOpen(group.key, open)">
            <CollapsibleTrigger as-child>
              <button
                class="text-sidebar-foreground/70 hover:text-sidebar-foreground focus-visible:ring-ring flex h-8 w-full shrink-0 items-center justify-between rounded-xl px-3 text-xs font-medium outline-hidden focus-visible:ring-2 group-data-[collapsible=icon]:hidden"
                :aria-expanded="groupOpen(group.key)"
              >
                <span>{{ group.label }}</span>
                <ChevronDown :class="['size-4 shrink-0 transition-transform', groupOpen(group.key) ? undefined : '-rotate-90']" aria-hidden="true" />
              </button>
            </CollapsibleTrigger>
            <!-- The caption sits --space-2 above its first item. -->
            <CollapsibleContent class="mt-2">
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem v-for="entry in group.entries" :key="entry.key">
                    <SidebarMenuButton :is-active="activePage === entry.key" :tooltip="entry.title" @click="go(entry.key)">
                      <component :is="entry.icon" aria-hidden="true" />
                      <span>{{ entry.title }}</span>
                      <SidebarMenuBadge v-if="badge(entry)">{{ badge(entry) }}</SidebarMenuBadge>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </CollapsibleContent>
          </Collapsible>
        </SidebarGroup>
      </template>
      <template v-else>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="Home" @click="goHome()">
                  <House aria-hidden="true" />
                  <span>Home</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup v-if="rtGroups.length === 0">
          <SidebarGroupContent>
            <p class="text-muted-foreground px-2 py-1 text-xs">No saved Record Types.</p>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup v-for="group in rtGroups" :key="group.key">
          <Collapsible :open="groupOpen(group.key)" @update:open="(open: boolean) => setGroupOpen(group.key, open)">
            <CollapsibleTrigger as-child>
              <button
                class="text-sidebar-foreground/70 hover:text-sidebar-foreground focus-visible:ring-ring flex h-8 w-full shrink-0 items-center justify-between rounded-xl px-3 text-xs font-medium outline-hidden focus-visible:ring-2 group-data-[collapsible=icon]:hidden"
                :aria-expanded="groupOpen(group.key)"
              >
                <span>{{ group.label }}</span>
                <ChevronDown :class="['size-4 shrink-0 transition-transform', groupOpen(group.key) ? undefined : '-rotate-90']" aria-hidden="true" />
              </button>
            </CollapsibleTrigger>
            <!-- The caption sits --space-2 above its first item. -->
            <CollapsibleContent class="mt-2">
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem v-for="recordType in group.types" :key="recordType.name">
                    <SidebarMenuButton :is-active="recordType.name === activeRecordType" :tooltip="recordType.label" @click="goRecordType(recordType.name)">
                      <component :is="recordTypeIcon" aria-hidden="true" />
                      <span>{{ recordType.label }}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </CollapsibleContent>
          </Collapsible>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem v-for="entry in viewStubEntries" :key="entry.key">
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
