// ImageField is the §7 Image input: a picker with a thumbnail preview, the
// required alt text, replace, download and remove. Choosing a file uploads it
// at once through PutImage; the value (path, hash, alt text) saves with the
// record. The uploaded file stays in images/ even when the value is removed
// or the record is deleted: files keep their own versions and audit.
<script setup lang="ts">
import { computed, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { fileClient } from '@/api'
import { describeImageUpload, imageDataUrl, uploadImage } from '@/images'
import type { ImageFieldValue } from '@/fieldRules'
import FieldShell from './FieldShell.vue'
import WorkspaceImage from '@/components/WorkspaceImage.vue'
import TextField from './TextField.vue'
import ActionButton from '@/components/ActionButton.vue'

const props = defineProps<{
  id: string
  label: string
  optional?: boolean
  help?: string
  error?: string
  disabled?: boolean
  modelValue: ImageFieldValue | null
  recordTypeKey: string
  fieldName: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: ImageFieldValue | null]; blur: [] }>()

const picker = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const downloading = ref(false)
const uploadError = ref('')
const shownError = computed(() => uploadError.value || props.error)

async function upload(file: File) {
  uploadError.value = ''
  uploading.value = true
  try {
    const uploaded = await uploadImage(`${props.recordTypeKey}-${props.fieldName}`, file)
    if ('error' in uploaded) {
      uploadError.value = uploaded.error
      return
    }
    emit('update:modelValue', { path: uploaded.path, hash: uploaded.hash, altText: props.modelValue?.altText ?? '' })
  } catch (err) {
    uploadError.value = describeImageUpload(err)
  } finally {
    uploading.value = false
  }
}

async function chosen(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file) await upload(file)
}

function updateAlt(altText: string) {
  if (props.modelValue) emit('update:modelValue', { ...props.modelValue, altText })
}

function remove() {
  uploadError.value = ''
  emit('update:modelValue', null)
  emit('blur')
}

async function downloadOriginal() {
  if (!props.modelValue || downloading.value) return
  downloading.value = true
  try {
    const res = await fileClient.getFile({ path: props.modelValue.path })
    const link = document.createElement('a')
    link.href = imageDataUrl(props.modelValue.path, res.content)
    link.download = props.modelValue.path.split('/').pop() ?? 'image'
    document.body.appendChild(link)
    link.click()
    link.remove()
  } catch (err) {
    uploadError.value = err instanceof ConnectError ? `The image could not be downloaded: ${err.rawMessage}` : 'The image could not be downloaded.'
  } finally {
    downloading.value = false
  }
}
</script>

<template>
  <FieldShell :id="id" :label="label" :optional="optional" :help="help" :error="shownError">
    <template #default="{ describedby, invalid }">
      <div class="image-field" :aria-describedby="describedby">
        <template v-if="modelValue">
          <WorkspaceImage :path="modelValue.path" thumbnail :alt="modelValue.altText || label" contain class="image-field-preview" />
          <TextField
            :id="`${id}-alt`"
            label="Alt text"
            :model-value="modelValue.altText"
            help="Describe the image for screen readers."
            :error="invalid && !modelValue.altText.trim() ? shownError : undefined"
            :disabled="disabled"
            placeholder="What the image shows"
            autocomplete="off"
            @update:model-value="updateAlt(String($event))"
            @blur="emit('blur')"
          />
          <div v-if="!disabled" class="image-field-actions" role="group" aria-label="Image actions">
            <ActionButton kind="secondary" density="row" label="Replace" icon="upload" :busy="uploading" @press="picker?.click()" />
            <ActionButton kind="quiet" density="row" label="Download" icon="download" :busy="downloading" @press="downloadOriginal" />
            <ActionButton kind="quiet" density="row" label="Remove" icon="x" @press="remove" />
          </div>
        </template>
        <div v-else class="image-field-empty">
          <ActionButton kind="secondary" density="row" label="Choose an image" icon="upload" :busy="uploading" :disabled-reason="disabled ? 'This field cannot be changed now.' : null" @press="picker?.click()" />
          <span class="muted-label">PNG, JPEG or WebP, at most 5 MB.</span>
        </div>
        <input ref="picker" class="files-picker" type="file" accept="image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp" :aria-label="`Upload an image for ${label}`" :disabled="disabled || uploading" @change="chosen" />
      </div>
    </template>
  </FieldShell>
</template>
