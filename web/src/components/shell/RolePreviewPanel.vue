// RolePreviewPanel shows View as one Role sees it (B8): the lists the
// Role's Grants reach, with the allowed actions per list. Read-only: it
// offers no record action and never shows Record contents. Builder words:
// previews run in Build mode.
<script setup lang="ts">
import { computed } from 'vue'
import { FileX2 } from '@/icons'
import { recordTypeIcon } from '@/pages/registry'
import ActionButton from '@/components/ActionButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import { roleView, type PreviewRole } from '@/composables/usePreview'
import type { RecordType } from '@/types'

const props = defineProps<{ role: PreviewRole; types: RecordType[] }>()
const emit = defineEmits<{ exit: [] }>()

const lists = computed(() => roleView(props.role, props.types))
</script>

<template>
  <section class="page-content">
    <div class="page-title-row">
      <div>
        <h1>View as {{ role.name }}</h1>
        <p class="lead">A read-only preview of the lists this Role sees. Record contents are not shown.</p>
      </div>
      <ActionButton kind="secondary" label="Exit preview" icon="x" @press="emit('exit')" />
    </div>
    <EmptyState
      v-if="lists.length === 0"
      :icon="FileX2"
      title="This Role sees no lists"
      body="Its Grants reach no saved Record Type."
    />
    <ul v-else class="preview-lists">
      <li v-for="list in lists" :key="list.name" class="preview-list">
        <component :is="recordTypeIcon" class="size-4 shrink-0" aria-hidden="true" />
        <div class="min-w-0">
          <p class="font-medium">{{ list.label }}</p>
          <p class="text-muted-foreground text-caption">{{ list.actions.join(', ') }}</p>
          <p v-if="list.hiddenFields > 0" class="text-muted-foreground text-caption">
            {{ list.hiddenFields }} {{ list.hiddenFields === 1 ? 'field stays' : 'fields stay' }} hidden.
          </p>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.preview-lists {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  list-style: none;
  margin: 0;
  padding: 0;
}
.preview-list {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--card);
  padding: var(--space-3) var(--space-4);
}
.preview-list > svg {
  margin-top: var(--space-1);
  color: var(--app-text-muted);
}
</style>
