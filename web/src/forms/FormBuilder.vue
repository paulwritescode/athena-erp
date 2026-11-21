// FormBuilder is the editor's Form tab (B9): sections with 1 or 2 columns,
// drag-and-drop ordering with full keyboard equivalents (move up, move down,
// move to section), per-field and per-section settings in a right sheet,
// static text and image blocks, art blocks, and a live preview at phone and
// desktop widths. It edits the draft layout in place; the editor's Save keeps
// it through the existing save path, and the journaled save shows in History.
<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import type { Field, RecordType } from '../types'
import type { FormLayout, LayoutItem, LayoutSection } from './layout'
import { addItem, addSection, arrangeAll, deleteSection, moveItem, moveItemTo, moveSection, removeItem, unplacedFields } from './editing'
import FormPreview from './FormPreview.vue'
import LayoutItemSheet, { type LayoutSelection } from './LayoutItemSheet.vue'
import ActionButton from '@/components/ActionButton.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import EmptyState from '@/components/EmptyState.vue'
import { Input } from '@/components/ui/input'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Frame, GripVertical, Image as ImageIcon, Monitor, Smartphone, Type as TypeIcon } from '@/icons'
import { icon } from '@/icons'

const props = defineProps<{ recordType: RecordType }>()

// Dragging is a fine-pointer affordance; touch and keyboard use the move
// buttons, which are always visible (§13).
const canDrag = useMediaQuery('(pointer: fine)')

const layout = computed(() => props.recordType.layout as FormLayout | undefined)
const hasSections = computed(() => (layout.value?.sections.length ?? 0) > 0)
const unplaced = computed(() => unplacedFields(layout.value, props.recordType.fields))

function draft(): FormLayout {
  if (!props.recordType.layout) props.recordType.layout = { sections: [] }
  return props.recordType.layout
}

function sectionName(section: LayoutSection, index: number): string {
  return section.title.trim() || `Section ${index + 1}`
}

function fieldOf(key: string): Field | undefined {
  return props.recordType.fields.find((field) => field.name === key)
}

function itemLabel(item: LayoutItem): string {
  switch (item.kind) {
    case 'field':
      return fieldOf(item.fieldKey)?.label ?? 'Removed field'
    case 'text':
      return 'Text block'
    case 'image':
      return 'Image block'
    case 'art':
      return 'Art block'
  }
}

function itemCaption(item: LayoutItem): string {
  switch (item.kind) {
    case 'field': {
      const field = fieldOf(item.fieldKey)
      return field ? `${field.type} · ${item.width === 'half' ? 'half' : 'full'} width` : item.fieldKey
    }
    case 'text':
      return item.text.trim().slice(0, 60) || 'Empty'
    case 'image':
      return item.path || 'No image chosen'
    case 'art':
      return item.key || 'No key yet'
  }
}

// Announcements for every reorder, so keyboard and screen-reader use stays
// oriented without seeing the drag animation.
const announce = ref('')
function say(text: string) {
  announce.value = ''
  void nextTick(() => {
    announce.value = text
  })
}

function focusRow(section: number, index: number, action: string) {
  void nextTick(() => {
    document.querySelector<HTMLElement>(`[data-row="${section}:${index}"] [data-action="${action}"]`)?.focus()
  })
}

function focusSection(index: number, action: string) {
  void nextTick(() => {
    document.querySelector<HTMLElement>(`[data-section="${index}"] [data-action="${action}"]`)?.focus()
  })
}

// The settings sheet edits one selected section, field or block.
const selection = ref<LayoutSelection | null>(null)
const sheetOpen = ref(false)
function edit(selectionNext: LayoutSelection) {
  selection.value = selectionNext
  sheetOpen.value = true
}

// Sections.
const confirmingDeleteSection = ref<number | null>(null)

function arrange() {
  arrangeAll(draft(), props.recordType.fields)
  say(`Arranged ${props.recordType.fields.length} fields into one section.`)
}

function appendSection() {
  const at = addSection(draft())
  say(`Section ${at + 1} added.`)
  focusSection(at, 'title')
}

function nudgeSection(index: number, direction: -1 | 1) {
  const current = layout.value
  if (!current) return
  const name = sectionName(current.sections[index] as LayoutSection, index)
  const next = moveSection(current, index, direction)
  if (next >= 0) {
    say(`${name} moved ${direction < 0 ? 'up' : 'down'}.`)
    // Focus stays on a working button: the same direction, or its opposite at
    // the edge, where the same one just disabled itself.
    const last = current.sections.length - 1
    focusSection(next, direction < 0 ? (next === 0 ? 'down' : 'up') : next === last ? 'up' : 'down')
  }
}

function removeSection(index: number) {
  const current = layout.value
  if (!current) return
  const name = sectionName(current.sections[index] as LayoutSection, index)
  const freed = (current.sections[index]?.items.filter((item) => item.kind === 'field') ?? []).length
  deleteSection(current, index)
  confirmingDeleteSection.value = null
  say(freed > 0 ? `${name} removed. ${freed} ${freed === 1 ? 'field is' : 'fields are'} unplaced.` : `${name} removed.`)
}

function selectColumns(index: number, next: unknown) {
  const target = layout.value?.sections[index]
  if (target && (next === '1' || next === '2')) target.columns = Number(next)
}

// Items.
function placeField(fieldKey: string, sectionIndex: number) {
  const current = layout.value
  const field = fieldOf(fieldKey)
  if (!current || !field) return
  addItem(current, sectionIndex, { kind: 'field', fieldKey, width: 'full', helpText: '' })
  say(`${field.label} added to ${sectionName(current.sections[sectionIndex] as LayoutSection, sectionIndex)}.`)
}

function addBlock(sectionIndex: number, kind: 'text' | 'image' | 'art') {
  const current = layout.value
  if (!current) return
  const item: LayoutItem = kind === 'text' ? { kind: 'text', text: '' } : kind === 'image' ? { kind: 'image', path: '' } : { kind: 'art', key: '' }
  const at = addItem(current, sectionIndex, item)
  say(`${itemLabel(item)} added to ${sectionName(current.sections[sectionIndex] as LayoutSection, sectionIndex)}.`)
  edit({ kind, section: sectionIndex, item: at })
}

function nudgeItem(sectionIndex: number, itemIndex: number, direction: -1 | 1) {
  const current = layout.value
  const item = current?.sections[sectionIndex]?.items[itemIndex]
  if (!current || !item) return
  const next = moveItem(current, sectionIndex, itemIndex, direction)
  if (next >= 0) {
    say(`${itemLabel(item)} moved ${direction < 0 ? 'up' : 'down'}.`)
    const last = (current.sections[sectionIndex]?.items.length ?? 1) - 1
    focusRow(sectionIndex, next, direction < 0 ? (next === 0 ? 'down' : 'up') : next === last ? 'up' : 'down')
  }
}

function sendItemTo(sectionIndex: number, itemIndex: number, toSection: number) {
  const current = layout.value
  const item = current?.sections[sectionIndex]?.items[itemIndex]
  if (!current || !item) return
  const to = current.sections[toSection]
  if (!to) return
  const landed = moveItemTo(current, sectionIndex, itemIndex, toSection, to.items.length)
  say(`${itemLabel(item)} moved to ${sectionName(to, toSection)}.`)
  focusRow(landed.section, landed.index, 'settings')
}

function detachItem(sectionIndex: number, itemIndex: number) {
  const current = layout.value
  const item = current?.sections[sectionIndex]?.items[itemIndex]
  if (!current || !item) return
  const label = itemLabel(item)
  removeItem(current, sectionIndex, itemIndex)
  say(item.kind === 'field' ? `${label} unplaced.` : `${label} removed.`)
}

// Drag and drop (fine pointers only). Items move within and across sections;
// sections reorder among themselves. The drop target is an insertion index:
// before row i, or after the last row.
type DragPayload = { kind: 'item'; section: number; item: number } | { kind: 'section'; section: number }
const dragging = ref<DragPayload | null>(null)
const dropTarget = ref<{ section: number; index: number } | null>(null)
const dropSection = ref<number | null>(null)

function dragType(event: DragEvent): string | null {
  const types = [...(event.dataTransfer?.types ?? [])]
  if (types.includes('application/x-athena-layout-item')) return 'item'
  if (types.includes('application/x-athena-layout-section')) return 'section'
  return null
}

function startItemDrag(section: number, item: number, event: DragEvent) {
  event.dataTransfer?.setData('application/x-athena-layout-item', JSON.stringify({ section, item }))
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  dragging.value = { kind: 'item', section, item }
}

function startSectionDrag(section: number, event: DragEvent) {
  event.dataTransfer?.setData('application/x-athena-layout-section', JSON.stringify({ section }))
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  dragging.value = { kind: 'section', section }
}

function overItemRow(section: number, item: number, event: DragEvent) {
  if (dragType(event) !== 'item') return
  event.preventDefault()
  event.stopPropagation()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  const row = event.currentTarget as HTMLElement
  const rect = row.getBoundingClientRect()
  dropTarget.value = { section, index: event.clientY - rect.top < rect.height / 2 ? item : item + 1 }
}

function overItemsEnd(section: number, event: DragEvent) {
  if (dragType(event) !== 'item') return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  dropTarget.value = { section, index: layout.value?.sections[section]?.items.length ?? 0 }
}

function dropItem(event: DragEvent) {
  const payload = dragging.value
  const target = dropTarget.value
  dragging.value = null
  dropTarget.value = null
  if (!payload || payload.kind !== 'item' || !target) return
  event.preventDefault()
  const current = layout.value
  const item = current?.sections[payload.section]?.items[payload.item]
  if (!current || !item) return
  // Dropping onto its own slot changes nothing.
  if (payload.section === target.section && (payload.item === target.index || payload.item + 1 === target.index)) return
  const landed = moveItemTo(current, payload.section, payload.item, target.section, target.index)
  const where = landed.section === payload.section ? '' : ` in ${sectionName(current.sections[landed.section] as LayoutSection, landed.section)}`
  say(`${itemLabel(item)} moved${where}.`)
}

function overSectionCard(section: number, event: DragEvent) {
  if (dragType(event) !== 'section') return
  event.preventDefault()
  event.stopPropagation()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  const card = event.currentTarget as HTMLElement
  const rect = card.getBoundingClientRect()
  dropSection.value = event.clientY - rect.top < rect.height / 2 ? section : section + 1
}

function dropSectionAt(event: DragEvent) {
  const payload = dragging.value
  const target = dropSection.value
  dragging.value = null
  dropSection.value = null
  if (!payload || payload.kind !== 'section' || target === null) return
  event.preventDefault()
  const current = layout.value
  const moving = current?.sections[payload.section]
  if (!current || !moving) return
  if (payload.section === target || payload.section + 1 === target) return
  current.sections.splice(payload.section, 1)
  current.sections.splice(payload.section < target ? target - 1 : target, 0, moving)
  say(`${sectionName(moving, target)} moved.`)
}

function endDrag() {
  dragging.value = null
  dropTarget.value = null
  dropSection.value = null
}

// The live preview beside the builder.
const previewWidth = ref<'phone' | 'desktop'>('desktop')
function selectPreviewWidth(next: unknown) {
  if (next === 'phone' || next === 'desktop') previewWidth.value = next
}
</script>

<template>
  <div class="form-builder">
    <div class="form-builder-main">
      <EmptyState
        v-if="!hasSections"
        :icon="icon('layout-template')"
        title="Arrange this form"
        body="Group the fields into sections with one or two columns, then add notes, images and artwork around them."
      >
        <template #action>
          <ActionButton kind="secondary" label="Arrange fields" icon="layout-template" @press="arrange" />
        </template>
      </EmptyState>
      <template v-else-if="layout">
        <div v-if="unplaced.length > 0" class="unplaced-tray" aria-label="Fields not placed yet">
          <p class="unplaced-title">Not placed yet ({{ unplaced.length }})</p>
          <ul>
            <li v-for="field in unplaced" :key="field.name" class="unplaced-row">
              <span class="unplaced-label">{{ field.label }}</span>
              <span class="muted-label">{{ field.type }}</span>
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button variant="outline" size="sm">Add to section</Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem v-for="(target, targetIndex) in layout.sections" :key="targetIndex" @select="placeField(field.name, targetIndex)">
                    {{ sectionName(target, targetIndex) }}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </li>
          </ul>
        </div>

        <div
          v-for="(section, sectionIndex) in layout.sections"
          :key="sectionIndex"
          class="form-builder-section"
          :data-section="sectionIndex"
          :draggable="canDrag"
          @dragstart="startSectionDrag(sectionIndex, $event)"
          @dragover="overSectionCard(sectionIndex, $event)"
          @drop="dropSectionAt($event)"
          @dragend="endDrag"
        >
          <div v-if="dropSection === sectionIndex" class="drop-indicator" aria-hidden="true" />
          <div class="builder-section-head">
            <GripVertical class="drag-handle size-4" aria-hidden="true" />
            <Input
              :model-value="section.title"
              :placeholder="`Section ${sectionIndex + 1}`"
              :aria-label="`Section ${sectionIndex + 1} title`"
              data-action="title"
              class="builder-section-title"
              @update:model-value="section.title = String($event)"
            />
            <ToggleGroup type="single" :model-value="String(section.columns)" :aria-label="`Section ${sectionIndex + 1} columns`" @update:model-value="selectColumns(sectionIndex, $event)">
              <ToggleGroupItem value="1" :aria-label="`Section ${sectionIndex + 1}: one column`">1</ToggleGroupItem>
              <ToggleGroupItem value="2" :aria-label="`Section ${sectionIndex + 1}: two columns`">2</ToggleGroupItem>
            </ToggleGroup>
            <div class="builder-row-actions" role="group" :aria-label="`Section ${sectionIndex + 1} actions`">
              <ActionButton kind="quiet" density="inline" icon-only :label="`Move section ${sectionIndex + 1} up`" icon="arrow-up" data-action="up" :disabled-reason="sectionIndex === 0 ? 'Already at the top.' : null" @press="nudgeSection(sectionIndex, -1)" />
              <ActionButton kind="quiet" density="inline" icon-only :label="`Move section ${sectionIndex + 1} down`" icon="arrow-down" data-action="down" :disabled-reason="sectionIndex === (layout?.sections.length ?? 0) - 1 ? 'Already at the bottom.' : null" @press="nudgeSection(sectionIndex, 1)" />
              <ActionButton kind="quiet" density="inline" icon-only :label="`Section ${sectionIndex + 1} settings`" icon="sliders-horizontal" data-action="settings" @press="edit({ kind: 'section', section: sectionIndex })" />
              <ActionButton kind="quiet" density="inline" icon-only :label="`Remove section ${sectionIndex + 1}`" icon="trash-2" @press="confirmingDeleteSection = sectionIndex" />
            </div>
          </div>

          <ul class="builder-items" @dragover="overItemsEnd(sectionIndex, $event)" @drop="dropItem($event)">
            <template v-for="(item, itemIndex) in section.items" :key="item.kind === 'field' ? `field-${item.fieldKey}` : `${item.kind}-${itemIndex}`">
              <div v-if="dropTarget?.section === sectionIndex && dropTarget.index === itemIndex" class="drop-indicator" aria-hidden="true" />
              <li
                class="builder-row"
                :data-row="`${sectionIndex}:${itemIndex}`"
                :draggable="canDrag"
                @dragstart="startItemDrag(sectionIndex, itemIndex, $event); $event.stopPropagation()"
                @dragover="overItemRow(sectionIndex, itemIndex, $event)"
                @drop="dropItem($event)"
                @dragend="endDrag"
              >
                <GripVertical class="drag-handle size-4" aria-hidden="true" />
                <span class="builder-row-label">
                  <span>{{ itemLabel(item) }}</span>
                  <span class="muted-label">{{ itemCaption(item) }}</span>
                </span>
                <div class="builder-row-actions" role="group" :aria-label="`${itemLabel(item)} actions`">
                  <ActionButton kind="quiet" density="inline" icon-only :label="`Move ${itemLabel(item)} up`" icon="arrow-up" data-action="up" :disabled-reason="itemIndex === 0 ? 'Already at the top.' : null" @press="nudgeItem(sectionIndex, itemIndex, -1)" />
                  <ActionButton kind="quiet" density="inline" icon-only :label="`Move ${itemLabel(item)} down`" icon="arrow-down" data-action="down" :disabled-reason="itemIndex === section.items.length - 1 ? 'Already at the bottom.' : null" @press="nudgeItem(sectionIndex, itemIndex, 1)" />
                  <DropdownMenu>
                    <DropdownMenuTrigger as-child>
                      <Button variant="ghost" size="icon" class="size-7" :aria-label="`Move ${itemLabel(item)} to another section`" data-action="move">
                        <component :is="icon('arrow-up-down')" class="size-4" aria-hidden="true" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem v-for="(target, targetIndex) in (layout?.sections ?? [])" :key="targetIndex" :disabled="targetIndex === sectionIndex" @select="sendItemTo(sectionIndex, itemIndex, targetIndex)">
                        {{ sectionName(target, targetIndex) }}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                  <ActionButton
                    v-if="!(item.kind === 'field' && !fieldOf(item.fieldKey))"
                    kind="quiet"
                    density="inline"
                    icon-only
                    :label="`${itemLabel(item)} settings`"
                    icon="sliders-horizontal"
                    data-action="settings"
                    @press="edit({ kind: item.kind, section: sectionIndex, item: itemIndex })"
                  />
                  <ActionButton kind="quiet" density="inline" icon-only :label="item.kind === 'field' ? `Unplace ${itemLabel(item)}` : `Remove ${itemLabel(item)}`" icon="x" data-action="remove" @press="detachItem(sectionIndex, itemIndex)" />
                </div>
              </li>
            </template>
            <div v-if="dropTarget?.section === sectionIndex && dropTarget.index === section.items.length" class="drop-indicator" aria-hidden="true" />
            <li class="builder-add-row">
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button variant="ghost" size="sm"><component :is="icon('plus')" class="size-4" aria-hidden="true" />Add block</Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuItem @select="addBlock(sectionIndex, 'text')"><TypeIcon class="size-4" aria-hidden="true" /><span>Text</span></DropdownMenuItem>
                  <DropdownMenuItem @select="addBlock(sectionIndex, 'image')"><ImageIcon class="size-4" aria-hidden="true" /><span>Image</span></DropdownMenuItem>
                  <DropdownMenuItem @select="addBlock(sectionIndex, 'art')"><Frame class="size-4" aria-hidden="true" /><span>Artwork</span></DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </li>
          </ul>
          <div v-if="dropSection === sectionIndex + 1" class="drop-indicator" aria-hidden="true" />
        </div>

        <ActionButton kind="quiet" label="Add section" icon="plus" @press="appendSection" />
      </template>
    </div>

    <div class="form-builder-preview">
      <div class="builder-preview-head">
        <h3>Preview</h3>
        <ToggleGroup type="single" :model-value="previewWidth" aria-label="Preview width" @update:model-value="selectPreviewWidth">
          <ToggleGroupItem value="phone" aria-label="Phone width"><Smartphone class="size-4" aria-hidden="true" /><span>Phone</span></ToggleGroupItem>
          <ToggleGroupItem value="desktop" aria-label="Desktop width"><Monitor class="size-4" aria-hidden="true" /><span>Desktop</span></ToggleGroupItem>
        </ToggleGroup>
      </div>
      <div class="builder-preview-frame" :class="previewWidth === 'phone' ? 'is-phone' : 'is-desktop'">
        <FormPreview :record-type-key="recordType.name" :fields="recordType.fields" :layout="layout" />
      </div>
    </div>

    <LayoutItemSheet v-model:open="sheetOpen" :record-type="recordType" :selection="selection" />
    <ConfirmDialog
      :open="confirmingDeleteSection !== null"
      :title="`Remove section ${(confirmingDeleteSection ?? 0) + 1}?`"
      body="Its fields return to unplaced. Nothing stored is deleted."
      confirm-label="Remove section"
      confirm-icon="trash-2"
      danger
      @update:open="confirmingDeleteSection = $event ? confirmingDeleteSection : null"
      @confirm="confirmingDeleteSection !== null && removeSection(confirmingDeleteSection)"
    />
    <p class="sr-only" role="status">{{ announce }}</p>
  </div>
</template>
