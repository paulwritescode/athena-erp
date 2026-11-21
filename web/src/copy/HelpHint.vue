// HelpHint explains one glossary term: the circle-help icon from section 5,
// opening a popover on desktop and a bottom drawer on touch (section 13.2).
// Placement rule: at most one next to a section or field title, only where
// the term is not self-explanatory, never inside a sentence or a page lead.
// The button always carries an accessible name, so the icon never stands
// alone.
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { CircleHelp } from '@/icons'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { gloss, type GlossaryKey } from './glossary'

const props = defineProps<{ term: GlossaryKey }>()

const open = ref(false)
const isDesktop = useMediaQuery('(min-width: 1024px)')
const entry = computed(() => gloss(props.term))
const accessibleName = computed(() => `What does ${entry.value.label} mean?`)
</script>

<template>
  <Popover v-if="isDesktop" :open="open" @update:open="open = $event">
    <PopoverTrigger as-child>
      <Button variant="ghost" size="icon-xs" class="help-hint" :aria-label="accessibleName">
        <CircleHelp class="size-4" aria-hidden="true" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="start" class="w-72">
      <p class="text-sm font-medium">{{ entry.label }}</p>
      <p class="text-muted-foreground mt-1 text-sm">{{ entry.explanation }}</p>
    </PopoverContent>
  </Popover>
  <template v-else>
    <Button variant="ghost" size="icon-xs" class="help-hint" :aria-label="accessibleName" @click="open = true">
      <CircleHelp class="size-4" aria-hidden="true" />
    </Button>
    <BottomDrawer :open="open" :title="entry.label" @update:open="open = $event">
      <p class="text-sm">{{ entry.explanation }}</p>
    </BottomDrawer>
  </template>
</template>

<style scoped>
.help-hint {
  display: inline-flex;
  vertical-align: baseline;
}
</style>
