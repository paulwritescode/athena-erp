// Notifications (stub): the bell and its panel. Real notifications land in a
// later batch; until then the panel says the user is caught up. Below 1024 px
// the panel is a bottom drawer (§17).
<script setup lang="ts">
import { ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { Bell } from '@/icons'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

const isDesktop = useMediaQuery('(min-width: 1024px)')
const open = ref(false)
</script>

<template>
  <Popover v-if="isDesktop" v-model:open="open">
    <PopoverTrigger as-child>
      <Button variant="ghost" size="icon-sm" aria-label="Notifications">
        <Bell class="size-4" aria-hidden="true" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="end" class="w-80">
      <p class="text-sm font-medium">Notifications</p>
      <p class="text-muted-foreground mt-1 text-sm">You're all caught up. Mentions and approvals will appear here.</p>
    </PopoverContent>
  </Popover>
  <template v-else>
    <Button variant="ghost" size="icon-sm" aria-label="Notifications" @click="open = true">
      <Bell class="size-4" aria-hidden="true" />
    </Button>
    <BottomDrawer v-model:open="open" title="Notifications" description="You're all caught up. Mentions and approvals will appear here." />
  </template>
</template>
