// LayoutItemSheet edits one selected thing of the form layout in a right
// sheet (a bottom drawer on phones): a section's title, description, columns
// and visibility rule, a field's width and help, or a text, image or art
// block's content. Edits apply to the draft at once; the editor's Save keeps
// them. Visibility is type-checked against the saved form through
// EvaluateExpression; the server checks again on save.
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { recordClient } from '@/api'
import type { RecordType } from '../types'
import type { FormLayout } from './layout'
import ArtPlaceholder from './ArtPlaceholder.vue'
import ImagePickerDialog from './ImagePickerDialog.vue'
import WorkspaceImage from '@/components/WorkspaceImage.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import ActionButton from '@/components/ActionButton.vue'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

export type LayoutSelection = { kind: 'section' | 'field' | 'text' | 'image' | 'art'; section: number; item?: number }

const props = defineProps<{ recordType: RecordType; selection: LayoutSelection | null }>()
const open = defineModel<boolean>('open', { required: true })

const layout = computed(() => props.recordType.layout as FormLayout | undefined)
const section = computed(() => (props.selection ? layout.value?.sections[props.selection.section] : undefined))
const item = computed(() => (props.selection?.item === undefined ? undefined : section.value?.items[props.selection.item]))

const title = computed(() => {
  switch (props.selection?.kind) {
    case 'section': {
      const name = section.value?.title?.trim()
      return name ? `${name} settings` : 'Section settings'
    }
    case 'field': {
      const key = item.value?.kind === 'field' ? item.value.fieldKey : ''
      const label = props.recordType.fields.find((field) => field.name === key)?.label ?? key
      return label ? `${label} settings` : 'Field settings'
    }
    case 'text':
      return 'Text block'
    case 'image':
      return 'Image block'
    case 'art':
      return 'Art block'
    default:
      return 'Settings'
  }
})

// Visibility is checked against the saved form: an InvalidArgument answer is
// an inline error, anything else (a record only a builder could read fails
// another way) stays silent for the server to judge on save.
const visibilityProblem = ref('')
let visibilityRequest = 0
let visibilityTimer: ReturnType<typeof setTimeout> | undefined

async function checkVisibilityNow(expression: string) {
  const request = ++visibilityRequest
  visibilityProblem.value = ''
  if (!expression.trim() || props.recordType.revision === 0) return
  try {
    const res = await recordClient.evaluateExpression({ recordTypeKey: props.recordType.name, expression })
    if (request !== visibilityRequest) return
    if (res.resultType !== 'bool') visibilityProblem.value = 'The rule must return true or false.'
  } catch (err) {
    if (request !== visibilityRequest) return
    if (err instanceof ConnectError && err.code === Code.InvalidArgument) visibilityProblem.value = err.rawMessage
  }
}

function scheduleVisibilityCheck() {
  clearTimeout(visibilityTimer)
  visibilityTimer = setTimeout(() => { void checkVisibilityNow(section.value?.visibility ?? '') }, 400)
}

watch([open, () => props.selection], () => {
  clearTimeout(visibilityTimer)
  visibilityRequest++
  visibilityProblem.value = ''
  if (open.value && props.selection?.kind === 'section' && (section.value?.visibility ?? '').trim()) {
    void checkVisibilityNow(section.value?.visibility ?? '')
  }
})

function setSection(patch: Partial<{ title: string; description: string; columns: number; visibility: string }>) {
  const target = section.value
  if (!target) return
  if (patch.title !== undefined) target.title = patch.title
  if (patch.description !== undefined) target.description = patch.description
  if (patch.columns !== undefined) target.columns = patch.columns
  if (patch.visibility !== undefined) {
    target.visibility = patch.visibility
    scheduleVisibilityCheck()
  }
}

function selectColumns(next: unknown) {
  if (next === '1' || next === '2') setSection({ columns: Number(next) })
}

function selectWidth(next: unknown) {
  if (item.value?.kind !== 'field') return
  if (next === 'full' || next === 'half') item.value.width = next
}

const pickingImage = ref(false)
function chooseImage(path: string) {
  if (item.value?.kind === 'image') item.value.path = path
  pickingImage.value = false
}
</script>

<template>
  <DetailSheet v-model:open="open" :title="title" description="Changes apply to the draft at once. Save keeps them.">
    <div v-if="selection?.kind === 'section' && section" class="settings-grid">
      <div class="settings-field"><Label for="layout-section-title">Title</Label><Input id="layout-section-title" :model-value="section.title" placeholder="Basics" @update:model-value="setSection({ title: String($event) })" /></div>
      <div class="settings-field"><Label for="layout-section-description">Description</Label><Textarea id="layout-section-description" :model-value="section.description" placeholder="One line saying what lives here." rows="2" @update:model-value="setSection({ description: String($event) })" /></div>
      <div class="settings-field">
        <Label id="layout-section-columns-label">Columns</Label>
        <ToggleGroup type="single" :model-value="String(section.columns)" aria-labelledby="layout-section-columns-label" @update:model-value="selectColumns">
          <ToggleGroupItem value="1" aria-label="One column">1</ToggleGroupItem>
          <ToggleGroupItem value="2" aria-label="Two columns">2</ToggleGroupItem>
        </ToggleGroup>
      </div>
      <div class="settings-field is-wide">
        <Label for="layout-section-visibility">Show when</Label>
        <Input id="layout-section-visibility" :model-value="section.visibility" class="font-mono" placeholder="doc.in_stock" spellcheck="false" autocomplete="off" autocapitalize="off" :aria-invalid="visibilityProblem ? 'true' : undefined" @update:model-value="setSection({ visibility: String($event) })" />
        <p v-if="visibilityProblem" class="field-error">{{ visibilityProblem }}</p>
        <p v-else class="settings-note">A rule over the record's fields, checked against the saved form. Empty always shows. Saving checks it again.</p>
      </div>
    </div>

    <div v-else-if="selection?.kind === 'field' && item?.kind === 'field'" class="settings-grid">
      <div class="settings-field">
        <Label id="layout-field-width-label">Width</Label>
        <ToggleGroup type="single" :model-value="item.width === 'half' ? 'half' : 'full'" aria-labelledby="layout-field-width-label" @update:model-value="selectWidth">
          <ToggleGroupItem value="full" aria-label="Full width">Full</ToggleGroupItem>
          <ToggleGroupItem value="half" aria-label="Half width">Half</ToggleGroupItem>
        </ToggleGroup>
        <p class="settings-note">Half takes one column of a two-column section; everything else spans it.</p>
      </div>
      <div class="settings-field is-wide"><Label for="layout-field-help">Help text</Label><Textarea id="layout-field-help" :model-value="item.helpText" placeholder="Shown under the field." rows="2" @update:model-value="item.helpText = String($event)" /></div>
    </div>

    <div v-else-if="selection?.kind === 'text' && item?.kind === 'text'" class="settings-grid">
      <div class="settings-field is-wide"><Label for="layout-text-body">Text</Label><Textarea id="layout-text-body" :model-value="item.text" placeholder="A note shown with the form." rows="4" @update:model-value="item.text = String($event)" /></div>
    </div>

    <div v-else-if="selection?.kind === 'image' && item?.kind === 'image'" class="settings-grid">
      <div class="settings-field is-wide">
        <Label id="layout-image-label">Image</Label>
        <div role="group" aria-labelledby="layout-image-label" class="layout-sheet-image-group">
          <WorkspaceImage v-if="item.path" :path="item.path" alt="" class="layout-sheet-image" />
          <p v-if="item.path" class="settings-note">{{ item.path }}</p>
          <ActionButton kind="secondary" density="row" :label="item.path ? 'Change image' : 'Choose image'" icon="image" @press="pickingImage = true" />
        </div>
      </div>
    </div>

    <div v-else-if="selection?.kind === 'art' && item?.kind === 'art'" class="settings-grid">
      <div class="settings-field is-wide"><Label for="layout-art-key">Artwork key</Label><Input id="layout-art-key" :model-value="item.key" class="font-mono" placeholder="stack" spellcheck="false" autocomplete="off" autocapitalize="off" @update:model-value="item.key = String($event)" /></div>
      <p class="settings-note is-wide">Free text for now; the art registry checks it when B10 lands.</p>
      <div class="is-wide"><ArtPlaceholder /></div>
    </div>

    <template #footer>
      <ActionButton kind="primary" label="Done" icon="check" @press="open = false" />
    </template>
  </DetailSheet>
  <ImagePickerDialog v-model:open="pickingImage" :current="item?.kind === 'image' ? item.path : ''" @choose="chooseImage" />
</template>
