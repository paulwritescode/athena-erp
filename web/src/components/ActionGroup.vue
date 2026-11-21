// ActionGroup lays actions out by kind per design-system section 14: --space-2
// between actions of one kind, --space-4 where the kind changes, and a
// hairline divider before a trailing destructive chunk. The caller owns the
// order; the group never reorders. With stackOnPhones the group becomes a
// column of full-width buttons below 640 px (a row's actions inside its
// detail drawer). Disabled reasons surface under the group on touch, where
// there is no hover for the tooltip.
<script setup lang="ts">
import { computed } from 'vue'
import ActionButton from '@/components/ActionButton.vue'
import { Separator } from '@/components/ui/separator'
import { chunkByKind, type ActionItem } from './actions'

const props = withDefaults(defineProps<{
  actions: ActionItem[]
  density?: 'page' | 'section' | 'row' | 'inline' | 'touch'
  stackOnPhones?: boolean
  ariaLabel?: string
}>(), {
  density: 'section',
  stackOnPhones: false,
})
const emit = defineEmits<{ press: [id: string] }>()

const chunks = computed(() => chunkByKind(props.actions))
const touchReasons = computed(() =>
  props.actions.filter((action) => action.disabledReason && !action.busy),
)
</script>

<template>
  <div v-if="actions.length > 0">
    <div
      role="group"
      :aria-label="ariaLabel"
      :class="['flex flex-wrap items-center gap-x-4 gap-y-2', stackOnPhones ? 'max-sm:flex-col max-sm:items-stretch max-sm:gap-x-0 max-sm:gap-y-2' : undefined]"
    >
      <template v-for="chunk in chunks" :key="chunk.kind + chunk.actions[0]?.id">
        <Separator v-if="chunk.dividerBefore" orientation="vertical" class="self-stretch max-sm:hidden" aria-hidden="true" />
        <div class="flex flex-wrap items-center gap-2" :class="stackOnPhones ? 'max-sm:flex-col max-sm:items-stretch' : undefined">
          <ActionButton
            v-for="action in chunk.actions"
            :key="action.id"
            :kind="chunk.kind"
            :density="density"
            :label="action.label"
            :icon="action.icon"
            :icon-only="action.iconOnly ?? false"
            :counter="action.counter"
            :shortcut="action.shortcut"
            :helper="action.helper"
            :busy="action.busy"
            :disabled-reason="action.disabledReason"
            :class="stackOnPhones ? 'max-sm:w-full' : undefined"
            @press="emit('press', action.id)"
          />
        </div>
      </template>
    </div>
    <ul v-if="touchReasons.length > 0" class="mt-2 hidden space-y-1 pointer-coarse:block">
      <li v-for="action in touchReasons" :key="action.id" class="text-caption text-muted-foreground">
        {{ action.label }}: {{ action.disabledReason }}
      </li>
    </ul>
  </div>
</template>
