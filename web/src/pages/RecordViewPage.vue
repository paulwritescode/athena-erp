<script setup lang="ts">
import type { RecordDefinition } from '../types'
import { Button } from '@/components/ui/button'
defineProps<{ records: RecordDefinition[]; currentRecord: RecordDefinition; notice: string }>()
const emit = defineEmits<{ select: [name: string]; back: []; save: [] }>()
</script>
<template>
  <section class="page-content view-page"><div class="view-head"><div><div class="eyebrow">VIEW · RECORD EXPERIENCE</div><h1>{{ currentRecord.label || 'Record preview' }}</h1><p class="lead">This is how an entry for the selected Record will appear.</p></div><div class="preview-label">RECORD PREVIEW <span>· entry storage is not implemented</span></div></div>
    <div v-if="records.length === 0" class="empty-builder"><p>Create a Record in Build to preview its entry screen.</p><Button variant="outline" @click="emit('back')">Go to Build</Button></div>
    <div v-else class="view-layout"><aside class="view-menu"><strong>WORKSPACE</strong><button v-for="record in records" :key="record.name" :class="{ selected: currentRecord.name === record.name }" @click="emit('select', record.name)"><span class="list-icon">▤</span>{{ record.label }}</button></aside><div class="record-preview"><div class="record-top"><div><div class="breadcrumb-label">Records / {{ currentRecord.label }}</div><h2>New {{ currentRecord.label }}</h2></div><div><Button variant="outline" size="sm" @click="emit('back')">Back to Build</Button> <Button size="sm" @click="emit('save')">Save definition</Button></div></div><p class="record-description">{{ currentRecord.description }}</p>
      <div class="record-fields"><label v-for="(field, index) in currentRecord.fields" :key="field.name + index">{{ field.label }}<span v-if="field.required" class="required-mark">*</span><input v-if="['Text', 'Email', 'Phone', 'Number', 'Currency', 'Date', 'Link'].includes(field.type)" :type="field.type === 'Email' ? 'email' : field.type === 'Date' ? 'date' : field.type === 'Number' || field.type === 'Currency' ? 'number' : 'text'" :placeholder="`Enter ${field.label.toLowerCase()}`"/><select v-else-if="field.type === 'Select'"><option value="">Select {{ field.label.toLowerCase() }}</option><option>Option 1</option><option>Option 2</option></select><textarea v-else-if="field.type === 'Long text'" rows="4" :placeholder="`Enter ${field.label.toLowerCase()}`"/><input v-else type="checkbox"/></label></div><div v-if="notice" class="notice-row">{{ notice }}</div><div class="record-bottom"><span>Required fields are marked with <b>*</b></span><span>Record definition · {{ currentRecord.fields.length }} fields</span></div>
    </div></div>
  </section>
</template>
