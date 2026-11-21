// ImagePickerDialog chooses a static image block's file from the images/
// folder, or uploads one. It opens from the layout sheet, so it sits one tier
// above it on both halves (dialog over sheet, drawer over drawer).
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { fileClient } from '@/api'
import { describeImageUpload, uploadImage } from '@/images'
import { imageExtensions } from '@/fieldRules'
import WorkspaceImage from '@/components/WorkspaceImage.vue'
import ActionButton from '@/components/ActionButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import { icon } from '@/icons'
import type { FileInfo } from '@/gen/athena/v1/files_pb'

const props = defineProps<{ current: string }>()
const emit = defineEmits<{ choose: [path: string] }>()
const open = defineModel<boolean>('open', { required: true })

const files = ref<FileInfo[]>([])
const state = ref<'loading' | 'ready' | 'error'>('loading')
const uploading = ref(false)
const uploadError = ref('')
const picker = ref<HTMLInputElement | null>(null)

const images = computed(() => files.value.filter((file) => file.path.startsWith('images/') && imageExtensions.some((extension) => file.path.endsWith(extension))))

async function refresh() {
  state.value = 'loading'
  try {
    files.value = (await fileClient.listFiles({})).files
    state.value = 'ready'
  } catch {
    state.value = 'error'
  }
}

watch(open, (isOpen) => {
  if (isOpen) {
    uploadError.value = ''
    void refresh()
  }
})

async function chosen(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  uploadError.value = ''
  uploading.value = true
  try {
    const uploaded = await uploadImage('layout', file)
    if ('error' in uploaded) {
      uploadError.value = uploaded.error
      return
    }
    await refresh()
    emit('choose', uploaded.path)
  } catch (err) {
    uploadError.value = describeImageUpload(err)
    if (!(err instanceof ConnectError && err.code === Code.Canceled)) void refresh()
  } finally {
    uploading.value = false
  }
}

function choose(path: string) {
  emit('choose', path)
}
</script>

<template>
  <ResponsiveDialog v-model:open="open" title="Choose an image" description="Static blocks show an uploaded image from the images folder." wide>
    <LoadingSkeleton v-if="state === 'loading'" />
    <div v-else-if="state === 'error'" class="flex flex-wrap items-center gap-2">
      <p class="text-sm">The images did not load.</p>
      <ActionButton kind="secondary" density="row" label="Try again" icon="refresh-ccw" @press="refresh" />
    </div>
    <template v-else>
      <EmptyState
        v-if="images.length === 0"
        :icon="icon('image')"
        title="No images yet"
        body="Upload one to show it in the form."
      />
      <div v-else class="image-picker-grid" role="listbox" aria-label="Uploaded images">
        <button
          v-for="file in images"
          :key="file.path"
          type="button"
          role="option"
          :aria-selected="file.path === current"
          class="image-picker-choice"
          :class="{ 'is-current': file.path === current }"
          @click="choose(file.path)"
        >
          <WorkspaceImage :path="file.path" thumbnail :alt="file.path" />
          <span class="image-picker-path">{{ file.path }}</span>
        </button>
      </div>
      <p v-if="uploadError" class="field-error mt-2">{{ uploadError }}</p>
    </template>
    <template #footer>
      <ActionButton kind="secondary" label="Upload" icon="upload" :busy="uploading" @press="picker?.click()" />
      <input ref="picker" class="files-picker" type="file" accept="image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp" aria-label="Upload an image" @change="chosen" />
    </template>
  </ResponsiveDialog>
</template>
