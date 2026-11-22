// PreviewAsGroup is the read-only "Preview as" control (B8): a
// hairline-separated sidebar group where a builder picks a Role and the
// main area shows View as that Role sees it. It previews navigation only,
// never Record contents. Hidden on the icon rail: a select needs labels.
<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarSeparator } from '@/components/ui/sidebar'
import type { PreviewRole } from '@/composables/usePreview'

const props = defineProps<{
  roles: PreviewRole[]
  state: 'idle' | 'loading' | 'ready' | 'failed'
  allowed: boolean
  deniedReason: string
  activeName: string
}>()
const emit = defineEmits<{ select: [name: string]; exit: []; retry: [] }>()

function picked(value: unknown) {
  if (typeof value === 'string' && value) emit('select', value)
}
</script>

<template>
  <SidebarSeparator class="mt-auto group-data-[collapsible=icon]:hidden" />
  <SidebarGroup class="group-data-[collapsible=icon]:hidden">
    <SidebarGroupLabel>Preview as</SidebarGroupLabel>
    <SidebarGroupContent class="px-2">
      <p v-if="!allowed" class="text-muted-foreground px-1 py-1 text-caption">{{ deniedReason }}</p>
      <template v-else-if="state === 'failed'">
        <p class="text-destructive px-1 py-1 text-caption">Could not load Roles.</p>
        <ActionButton kind="quiet" label="Retry" icon="refresh-ccw" @press="emit('retry')" />
      </template>
      <Select v-else :model-value="activeName === '' ? undefined : activeName" :disabled="state !== 'ready' || roles.length === 0" @update:model-value="picked">
        <SelectTrigger class="w-full" :aria-label="activeName ? `Previewing as ${activeName}` : 'Preview as a Role'">
          <SelectValue :placeholder="state === 'loading' ? 'Loading Roles…' : roles.length === 0 ? 'No Roles yet' : 'Choose a Role…'" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="role in roles" :key="role.name" :value="role.name">{{ role.name }}</SelectItem>
        </SelectContent>
      </Select>
      <ActionButton v-if="activeName" kind="quiet" label="Exit preview" icon="x" class="mt-1" @press="emit('exit')" />
    </SidebarGroupContent>
  </SidebarGroup>
</template>
