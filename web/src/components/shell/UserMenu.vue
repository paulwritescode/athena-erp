// The signed-in user's menu: identity, theme, password, sign out. Below 640
// px the Build/View switch lives here because the header has no room for it.
<script setup lang="ts">
import { computed } from 'vue'
import { Check, ChevronDown, Moon, Sun } from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { SessionUser } from '@/types'

const props = defineProps<{ user: SessionUser; isDark: boolean; mode: 'build' | 'view'; canBuildDesk: boolean }>()
const emit = defineEmits<{ setMode: ['build' | 'view']; toggleTheme: []; changePassword: []; signOut: [] }>()

const name = computed(() => props.user.displayName || props.user.username)
const initials = computed(() => name.value.trim().split(/\s+/).map((part) => part[0]).slice(0, 2).join('').toUpperCase() || '?')
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="sm" class="gap-2 px-2" :aria-label="`Signed in as ${name}`">
        <Avatar size="sm">
          <AvatarFallback>{{ initials }}</AvatarFallback>
        </Avatar>
        <span class="hidden max-w-28 truncate min-[1024px]:inline">{{ name }}</span>
        <ChevronDown class="text-muted-foreground hidden size-3.5 min-[1024px]:inline" aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="w-56">
      <DropdownMenuLabel>
        <span class="block truncate font-medium">{{ name }}</span>
        <small class="text-muted-foreground block truncate font-normal">{{ user.owner ? 'Instance owner' : user.username }}</small>
      </DropdownMenuLabel>
      <template v-if="canBuildDesk">
        <DropdownMenuSeparator class="min-[640px]:hidden" />
        <DropdownMenuItem class="min-[640px]:hidden" @select="emit('setMode', 'build')">
          <span class="flex-1">Build mode</span>
          <Check v-if="mode === 'build'" class="size-4" aria-hidden="true" />
        </DropdownMenuItem>
        <DropdownMenuItem class="min-[640px]:hidden" @select="emit('setMode', 'view')">
          <span class="flex-1">View mode</span>
          <Check v-if="mode === 'view'" class="size-4" aria-hidden="true" />
        </DropdownMenuItem>
      </template>
      <DropdownMenuSeparator />
      <DropdownMenuItem @select="emit('toggleTheme')">
        <component :is="isDark ? Sun : Moon" class="size-4" aria-hidden="true" />
        <span>{{ isDark ? 'Light theme' : 'Dark theme' }}</span>
      </DropdownMenuItem>
      <DropdownMenuItem @select="emit('changePassword')">Change password</DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem @select="emit('signOut')">Sign out</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
