// The phone summary for one Record Type (§19): description, counts, Pack and
// last change, with Open editor and View records actions.
<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import BottomDrawer from '@/components/BottomDrawer.vue'
import { Badge } from '@/components/ui/badge'
import { kindLabel, packLabel } from './recordTypeList'
import { formatWhen } from '@/times'
import type { RecordType } from '@/types'

defineProps<{ recordType: RecordType | null }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ openEditor: [key: string]; viewRecords: [key: string] }>()

function openEditor(key: string) {
  open.value = false
  emit('openEditor', key)
}

function viewRecords(key: string) {
  open.value = false
  emit('viewRecords', key)
}
</script>

<template>
  <BottomDrawer v-model:open="open" :title="recordType?.label ?? 'Record Type'">
    <template v-if="recordType" #default>
      <div class="mb-3 flex items-center gap-2">
        <Badge variant="secondary">{{ kindLabel(recordType.kind || 'master') }}</Badge>
      </div>
      <p v-if="recordType.description" class="text-muted-foreground text-sm">{{ recordType.description }}</p>
      <dl class="mt-4 space-y-2 text-sm">
        <div class="flex items-center justify-between gap-4">
          <dt class="text-muted-foreground">Fields</dt>
          <dd class="tabular-nums">{{ recordType.fields.length }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-muted-foreground">Records</dt>
          <dd v-if="recordType.count_hidden" class="text-muted-foreground">Hidden</dd>
          <dd v-else class="tabular-nums">{{ recordType.record_count ?? 0 }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-muted-foreground">Pack</dt>
          <dd>{{ packLabel(recordType.pack_key) }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-muted-foreground">Last changed</dt>
          <dd>{{ recordType.updated_at ? formatWhen(recordType.updated_at) : '—' }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-muted-foreground">Changed by</dt>
          <dd>{{ recordType.updated_by || '—' }}</dd>
        </div>
      </dl>
    </template>
    <template #footer>
      <div class="flex flex-col gap-2">
        <ActionButton kind="primary" label="Open editor" icon="pencil" full-bleed @press="recordType && openEditor(recordType.name)" />
        <ActionButton kind="secondary" label="View records" icon="eye" full-bleed @press="recordType && viewRecords(recordType.name)" />
      </div>
    </template>
  </BottomDrawer>
</template>
