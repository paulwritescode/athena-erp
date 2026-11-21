// ModeSwitch flips the desk between Build and View. Single-select: clicking
// the active mode keeps it instead of deselecting.
<script setup lang="ts">
import { Eye, Hammer } from '@/icons'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const mode = defineModel<'build' | 'view'>({ required: true })

function select(next: unknown) {
  if (next === 'build' || next === 'view') mode.value = next
}
</script>

<template>
  <!-- Joined segmented control on the §15 radius family: a hairline 8 px
  container, square meeting in the middle, 7 px outer ends. The ends need
  important: the primitive squares joined items on a stacked variant. -->
  <ToggleGroup
    type="single"
    variant="outline"
    :model-value="mode"
    aria-label="Desk mode"
    class="border-border border data-[spacing=0]:data-[variant=outline]:rounded-[var(--radius-md)]"
    @update:model-value="select"
  >
    <ToggleGroupItem value="build" aria-label="Build mode" class="rounded-l-[calc(var(--radius-md)-1px)]! rounded-r-none!">
      <Hammer class="size-4" aria-hidden="true" />
      <span>Build</span>
    </ToggleGroupItem>
    <ToggleGroupItem value="view" aria-label="View mode" class="rounded-r-[calc(var(--radius-md)-1px)]! rounded-l-none!">
      <Eye class="size-4" aria-hidden="true" />
      <span>View</span>
    </ToggleGroupItem>
  </ToggleGroup>
</template>
