// Notifications (stub): the bell and its panel. Real notifications land in a
// later batch; until then the panel says the user is caught up.
<script setup lang="ts">
import { Bell } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
</script>

<template>
  <Popover>
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
</template>
