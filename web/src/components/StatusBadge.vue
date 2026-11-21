// StatusBadge reads one word of state: draft, submitted, paid. Well-known
// states map to badge variants; anything else renders as a quiet outline.
// Pass variant to say it differently.
<script setup lang="ts">
import { computed } from 'vue'
import { Badge, type BadgeVariants } from '@/components/ui/badge'

type Variant = NonNullable<BadgeVariants['variant']>

const props = defineProps<{ status: string; label?: string; variant?: Variant }>()

const known: Record<string, Variant> = {
  draft: 'secondary',
  pending: 'outline',
  submitted: 'success',
  active: 'success',
  paid: 'success',
  open: 'success',
  inactive: 'secondary',
  cancelled: 'destructive',
  failed: 'destructive',
  overdue: 'destructive',
}

const variant = computed<Variant>(() => props.variant ?? known[props.status.trim().toLowerCase()] ?? 'outline')
</script>

<template>
  <Badge :variant="variant">{{ label ?? status }}</Badge>
</template>
