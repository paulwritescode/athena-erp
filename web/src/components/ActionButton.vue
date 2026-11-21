// ActionButton is the one labelled action, per design-system section 14:
// six kinds (primary, secondary, quiet, caution, destructive, restore), five
// densities, an icon left of the label, and busy and disabled-with-reason
// states. It emits press, never click, so callers think in actions: the press
// fires from pointer, keyboard and the registered shortcut alike. Shortcuts
// register globally while the button lives; write them display-ready ('⌘K').
// Densities: page (hero actions), section (cards, drawers), row (table rows),
// inline (inside text and pills), touch (guaranteed 44 px). Coarse pointers
// and phone widths keep a 44 px floor on every density (see style.css).
// The disabled reason shows as a native tooltip on hover (the button itself
// is pointer-dead while disabled, so the tooltip lives on a wrapper span)
// and, inside an ActionGroup, as a line under the group on touch.
<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { Button } from '@/components/ui/button'
import { LoaderCircle, icon, type IconName } from '@/icons'
import { registerShortcut, unregisterShortcut } from '@/shortcuts'
import type { ActionKind } from './actions'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  kind?: ActionKind
  iconOnly?: boolean
  density?: 'page' | 'section' | 'row' | 'inline' | 'touch'
  label: string
  icon?: IconName
  counter?: number | string | null
  shortcut?: string
  helper?: string
  busy?: boolean
  disabledReason?: string | null
  fullBleed?: boolean
  type?: 'button' | 'submit' | 'reset'
}>(), {
  kind: 'secondary',
  iconOnly: false,
  density: 'section',
  busy: false,
  disabledReason: null,
  fullBleed: false,
  type: 'button',
})
const emit = defineEmits<{ press: [] }>()

// Primary and secondary map straight onto the shadcn variants; the rest are
// the outline shape with kind colors so destructive stays an outline at rest
// (the shadcn destructive variant is a tint, which section 14 forbids).
const buttonVariant = computed(() => {
  if (props.kind === 'primary') return 'default'
  if (props.kind === 'quiet' || props.iconOnly) return 'ghost'
  return 'outline'
})

const kindClass = computed(() => {
  if (props.iconOnly) return ''
  switch (props.kind) {
    case 'quiet':
      return 'text-muted-foreground hover:text-foreground'
    case 'caution':
      return 'border-warning/60 text-warning hover:border-warning hover:bg-warning-soft hover:text-warning'
    case 'destructive':
      return 'border-destructive/60 text-destructive hover:border-destructive hover:bg-destructive hover:text-destructive-foreground'
    default:
      return ''
  }
})

const densityClass = computed(() => {
  if (props.iconOnly) {
    return { page: 'size-10', section: 'size-9', row: 'size-8', inline: 'size-7', touch: 'size-11' }[props.density]
  }
  return {
    page: 'h-10 px-4',
    section: 'h-9 px-3',
    row: 'h-8 px-3',
    inline: 'h-7 px-2 text-caption',
    touch: 'h-11 px-4',
  }[props.density]
})

const disabled = computed(() => props.busy || props.disabledReason !== null)
const leadingIcon = computed(() => (props.icon ? icon(props.icon) : null))
// Restore tints only its icon; caution inherits the warning text; the
// destructive icon follows the text fill on hover via the button group.
const iconClass = computed(() => {
  if (props.kind === 'restore') return 'size-4 text-success'
  if (props.kind === 'destructive' && !props.iconOnly) return 'size-4 text-destructive group-hover/button:text-destructive-foreground'
  return 'size-4'
})
const showLabel = computed(() => !props.iconOnly)
// The hover tooltip: the disabled reason, or the name on icon-only buttons.
const tip = computed(() => {
  if (props.disabledReason && !props.busy) return props.disabledReason
  if (props.iconOnly) return props.label
  return null
})

function press() {
  if (disabled.value) return
  emit('press')
}

function onShortcutFired() {
  press()
}

onMounted(() => {
  if (props.shortcut) registerShortcut(props.shortcut, `ActionButton:${props.label}`, onShortcutFired)
})

onUnmounted(() => {
  unregisterShortcut(onShortcutFired)
})
</script>

<template>
  <span :title="tip ?? undefined" :class="tip ? ['inline-flex min-w-0', fullBleed ? 'w-full' : undefined] : 'contents'">
    <Button
      :variant="buttonVariant"
      :type="type"
      :disabled="disabled"
      :aria-label="($attrs['aria-label'] as string | undefined) ?? (iconOnly || disabledReason ? (disabledReason && !busy ? `${label}: ${disabledReason}` : label) : undefined)"
      :class="['action-button', 'gap-2', 'duration-athena-fast', 'motion-safe:active:scale-[0.97]', densityClass, kindClass, fullBleed ? 'w-full flex-1' : undefined]"
      v-bind="$attrs"
      @click="press"
    >
      <LoaderCircle v-if="busy" :class="iconClass" class="animate-spin" aria-hidden="true" />
      <component :is="leadingIcon ?? undefined" v-else-if="leadingIcon" :class="iconClass" aria-hidden="true" />
      <span v-if="showLabel">{{ label }}</span>
      <span v-if="counter != null && !iconOnly" class="rounded-full bg-muted px-1.5 py-0.5 text-xs font-medium tabular-nums">{{ counter }}</span>
      <kbd v-if="shortcut && !iconOnly" aria-hidden="true" class="rounded border bg-muted px-1 font-medium text-caption">{{ shortcut }}</kbd>
      <span v-if="helper && !iconOnly" class="text-caption opacity-70">{{ helper }}</span>
    </Button>
  </span>
</template>
