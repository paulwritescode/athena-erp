// ModeSwitch flips the desk between Build and View. Single-select: clicking
// the active mode keeps it instead of deselecting.
<script setup lang="ts">
import { Eye, Hammer } from '@lucide/vue'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const mode = defineModel<'build' | 'view'>({ required: true })

function select(next: unknown) {
  if (next === 'build' || next === 'view') mode.value = next
}
</script>

<template>
  <ToggleGroup type="single" variant="outline" :model-value="mode" aria-label="Desk mode" @update:model-value="select">
    <ToggleGroupItem value="build" aria-label="Build mode">
      <Hammer class="size-4" aria-hidden="true" />
      <span>Build</span>
    </ToggleGroupItem>
    <ToggleGroupItem value="view" aria-label="View mode">
      <Eye class="size-4" aria-hidden="true" />
      <span>View</span>
    </ToggleGroupItem>
  </ToggleGroup>
</template>
