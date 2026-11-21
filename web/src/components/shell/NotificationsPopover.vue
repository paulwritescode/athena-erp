// The notification bell and inbox (B12, §17): an unread count, Today and
// Earlier groups with read state, Mark all as read, and a deep link per
// item. A popover on desktop, a bottom drawer below 1024 px.
<script setup lang="ts">
import { onMounted } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Bell } from '@/icons'
import ActionButton from '@/components/ActionButton.vue'
import BottomDrawer from '@/components/BottomDrawer.vue'
import InboxBody from './InboxBody.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useNotifications, type NotificationsState } from '@/composables/useNotifications'
import type { Notification } from '../../gen/athena/v1/notifications_pb'
import type { NotificationTarget } from '../../notifications'

const emit = defineEmits<{ navigate: [target: NotificationTarget] }>()

const isDesktop = useMediaQuery('(min-width: 1024px)')
const inbox: NotificationsState = useNotifications()

onMounted(() => inbox.start())

function toggle(open: boolean) {
  if (open) void inbox.openInbox()
  else inbox.closeInbox()
}

function openItem(item: Notification) {
  const target = inbox.targetFor(item)
  void inbox.markRead([item.id])
  inbox.closeInbox()
  emit('navigate', target)
}
</script>

<template>
  <Popover v-if="isDesktop" :open="inbox.inboxOpen.value" @update:open="toggle">
    <PopoverTrigger as-child>
      <Button variant="ghost" size="icon-sm" aria-label="Notifications" class="relative">
        <Bell class="size-4" aria-hidden="true" />
        <Badge v-if="inbox.unread.value > 0" variant="destructive" class="absolute -top-1 -right-1 h-4 min-w-4 px-1 text-[10px] tabular-nums">
          {{ inbox.unread.value > 99 ? '99+' : inbox.unread.value }}
        </Badge>
      </Button>
    </PopoverTrigger>
    <PopoverContent align="end" class="max-h-[70vh] w-96 overflow-y-auto">
      <InboxBody :inbox="inbox" @open-item="openItem" @retry="inbox.openInbox()" />
    </PopoverContent>
  </Popover>
  <template v-else>
    <Button variant="ghost" size="icon-sm" aria-label="Notifications" class="relative" @click="toggle(true)">
      <Bell class="size-4" aria-hidden="true" />
      <Badge v-if="inbox.unread.value > 0" variant="destructive" class="absolute -top-1 -right-1 h-4 min-w-4 px-1 text-[10px] tabular-nums">
        {{ inbox.unread.value > 99 ? '99+' : inbox.unread.value }}
      </Badge>
    </Button>
    <BottomDrawer :open="inbox.inboxOpen.value" title="Notifications" @update:open="(next: boolean) => toggle(next)">
      <InboxBody :inbox="inbox" :title="false" @open-item="openItem" @retry="inbox.openInbox()" />
    </BottomDrawer>
  </template>
</template>
