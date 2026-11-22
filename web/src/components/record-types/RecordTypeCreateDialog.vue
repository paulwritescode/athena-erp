// The name-first creation dialog (§20): Name, purpose and Kind, with the
// derived key shown for approval. ResponsiveDialog renders it centered on
// desktop and as a bottom drawer on touch.
<script setup lang="ts">
import { computed, ref } from 'vue'
import ActionButton from '@/components/ActionButton.vue'
import HelpHint from '@/copy/HelpHint.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import TextField from '@/components/fields/TextField.vue'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { KIND_OPTIONS, deriveRecordTypeKey, validateNewRecordType } from './recordTypeKeys'
import type { NewRecordTypeInput } from '@/composables/useRecordTypes'
import type { Kind, RecordType } from '@/types'

const props = defineProps<{ recordTypes: RecordType[] }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ create: [input: NewRecordTypeInput, complete: (saved: boolean) => void] }>()

const name = ref('')
const description = ref('')
const kind = ref<Kind>('master')
const keyManual = ref<string | null>(null)
const parent = ref('')
const nameTouched = ref(false)
const keyTouched = ref(false)
const creating = ref(false)

const takenNames = computed(() => props.recordTypes.map((type) => type.name.toLowerCase()))
const takenLabels = computed(() => props.recordTypes.map((type) => type.label.trim().toLowerCase()))
const key = computed(() => keyManual.value ?? deriveRecordTypeKey(name.value, takenNames.value))
const errors = computed(() => validateNewRecordType(name.value, key.value, takenNames.value, takenLabels.value))
const parents = computed(() => props.recordTypes.filter((type) => type.revision > 0 && (type.kind === 'master' || type.kind === 'document')))
const parentError = computed(() => (kind.value === 'child' && !parent.value ? 'Choose the Record Type these records belong to.' : undefined))
const noParents = computed(() => kind.value === 'child' && parents.value.length === 0)
const dirty = computed(() => name.value !== '' || description.value !== '' || kind.value !== 'master' || keyManual.value !== null || parent.value !== '')
const blocked = computed(() => {
  if (errors.value.name) return errors.value.name
  if (errors.value.key) return errors.value.key
  if (parentError.value) return parentError.value
  return null
})

function toggleKeyManual() {
  if (keyManual.value === null) keyManual.value = key.value
  else {
    // Back to auto-derive: the derived key is always valid, so the Key
    // field's touched state goes with it.
    keyManual.value = null
    keyTouched.value = false
  }
}

function create() {
  if (blocked.value || creating.value) return
  creating.value = true
  nameTouched.value = true
  keyTouched.value = true
  emit('create', { name: name.value.trim(), description: description.value.trim(), kind: kind.value, key: key.value, parent: parent.value || undefined }, (saved: boolean) => {
    creating.value = false
    if (!saved) nameTouched.value = true
  })
}
</script>

<template>
  <ResponsiveDialog v-model:open="open" title="New Record Type" :dirty="dirty" :dismissible="!creating">
    <div class="space-y-4">
      <TextField
        id="new-type-name"
        v-model="name"
        label="Name"
        placeholder="Customer"
        autofocus
        :error="nameTouched ? errors.name : undefined"
        :disabled="creating"
        @blur="nameTouched = true"
      />
      <div class="text-muted-foreground -mt-2 text-sm">
        Saved as <code class="font-mono">{{ key || '—' }}</code>{{ ' ' }}
        <button class="cursor-pointer underline" @click="toggleKeyManual">{{ keyManual === null ? 'Change' : 'Auto' }}</button>
      </div>
      <TextField
        v-if="keyManual !== null"
        id="new-type-key"
        v-model="keyManual"
        label="Key"
        :error="keyTouched ? errors.key : undefined"
        :disabled="creating"
        @blur="keyTouched = true"
      />
      <TextField
        id="new-type-description"
        v-model="description"
        label="What is it for?"
        optional
        placeholder="What this kind of record tracks"
        :disabled="creating"
      />
      <fieldset>
        <legend class="text-sm font-medium">Kind <HelpHint term="recordKind" /></legend>
        <RadioGroup v-model="kind" class="mt-2 gap-3" :disabled="creating" aria-label="Kind">
          <div v-for="option in KIND_OPTIONS" :key="option.value" class="flex items-start gap-2">
            <RadioGroupItem :id="`new-type-kind-${option.value}`" :value="option.value" class="mt-0.5" />
            <Label :for="`new-type-kind-${option.value}`" class="font-normal">
              <span class="grid gap-0.5">
                <span class="font-medium leading-none">{{ option.label }}</span>
                <span class="text-muted-foreground text-sm">{{ option.hint }}</span>
              </span>
            </Label>
          </div>
        </RadioGroup>
      </fieldset>
      <div v-if="kind === 'child'" class="space-y-2">
        <Label for="new-type-parent" class="text-sm font-medium">Belongs to</Label>
        <NativeSelect v-model="parent" class="w-full" aria-label="Parent Record Type" :disabled="creating || parents.length === 0">
          <NativeSelectOption value="" disabled>Select a saved Record Type</NativeSelectOption>
          <NativeSelectOption v-for="item in parents" :key="item.name" :value="item.name">{{ item.label }}</NativeSelectOption>
        </NativeSelect>
        <p v-if="noParents" class="text-muted-foreground text-sm">Save a Master or Document first; a child belongs to one of its records.</p>
        <p v-else-if="parentError" class="text-destructive text-sm">{{ parentError }}</p>
      </div>
    </div>
    <template #footer>
      <ActionButton kind="secondary" label="Cancel" icon="x" :disabled="creating" @press="open = false" />
      <ActionButton kind="primary" label="Create" icon="plus" :busy="creating" :disabled-reason="blocked" @press="create" />
    </template>
  </ResponsiveDialog>
</template>
