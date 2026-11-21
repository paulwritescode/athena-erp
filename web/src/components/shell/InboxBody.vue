// The notification list shared by the desktop popover and the touch drawer.
<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import { Bell } from '@/icons'
import type { NotificationsState } from '@/composables/useNotifications'
import { formatWhen } from '../../times'
import type { Notification } from '../../gen/athena/v1/notifications_pb'

withDefaults(defineProps<{ inbox: NotificationsState; title?: boolean }>(), { title: true })
const emit = defineEmits<{ openItem: [item: Notification]; retry: [] }>()
</script>

<template>
  <div class="flex items-center gap-2" :class="title ? 'justify-between' : 'justify-end'">
    <p v-if="title" class="text-sm font-medium">Notifications</p>
    <ActionButton kind="quiet" density="inline" label="Mark all as read" icon="check" @press="inbox.markAllRead()" />
  </div>
  <div v-if="!inbox.loaded.value" class="mt-3 space-y-2" aria-label="Loading notifications">
    <div class="h-12 animate-pulse rounded-[var(--radius)] bg-muted" />
    <div class="h-12 animate-pulse rounded-[var(--radius)] bg-muted" />
  </div>
  <div v-else-if="inbox.loadError.value" class="mt-3 space-y-2">
    <p class="text-sm text-muted-foreground">{{ inbox.loadError.value }}</p>
    <ActionButton kind="secondary" density="row" label="Try again" icon="refresh-ccw" @press="emit('retry')" />
  </div>
  <div v-else-if="inbox.items.value.length === 0" class="mt-2">
    <EmptyState :icon="Bell" title="You're all caught up" body="Mentions, approvals and account notices will appear here." />
  </div>
  <div v-else class="mt-2 space-y-4">
    <section v-for="group in (['today', 'earlier'] as const)" :key="group">
      <div v-if="inbox.groups.value[group].length > 0">
        <p class="muted-label mb-1">{{ group === 'today' ? 'Today' : 'Earlier' }}</p>
        <ul class="space-y-1">
          <li v-for="item in inbox.groups.value[group]" :key="item.id">
            <button
              type="button"
              class="flex w-full items-start gap-2 rounded-[var(--radius)] p-2 text-left hover:bg-muted"
              @click="emit('openItem', item)"
            >
              <span
                class="mt-1.5 size-2 shrink-0 rounded-full"
                :class="item.readAt ? 'bg-transparent' : 'bg-primary'"
                :aria-label="item.readAt ? 'Read' : 'Unread'"
                role="img"
              />
              <span class="min-w-0 flex-1">
                <span class="block text-sm">{{ item.text }}</span>
                <span class="muted-label block truncate"><template v-if="item.actorLabel || item.subjectLabel">{{ item.actorLabel || item.subjectLabel }} · </template>{{ formatWhen(item.createdAt) }}</span>
              </span>
            </button>
          </li>
        </ul>
      </div>
    </section>
    <ActionButton
      v-if="inbox.nextToken.value"
      kind="secondary"
      density="row"
      label="Show older"
      icon="chevron-down"
      :busy="inbox.loadingMore.value"
      @press="inbox.showOlder()"
    />
  </div>
</template>
