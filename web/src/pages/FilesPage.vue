<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { fileClient } from '../api'
import type { FileInfo, FileVersion } from '../gen/athena/v1/files_pb'
import { fileExtensions, fileFolders, maxFileBytes, validateFilePath, withinFileLimit } from '../filePaths'
import ActionButton from '@/components/ActionButton.vue'
import ActionGroup from '@/components/ActionGroup.vue'
import type { ActionItem } from '@/components/actions'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import { formatWhen } from '../times'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import ResponsiveDialog from '@/components/ResponsiveDialog.vue'
import TextField from '@/components/fields/TextField.vue'

// The Files page manages the Workspace files through FileService. The server
// checks every path, size and type, and Record Type files against their file
// format; filePaths.ts only repeats the path rules so a mistake shows early.
const props = defineProps<{ canWrite: boolean }>()

const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true })

const files = ref<FileInfo[]>([])
const withoutFile = ref<string[]>([])
const loaded = ref(false)
const unavailable = ref('')
const collapsed = ref(new Set<string>())

const selected = ref('')
const text = ref('')
const savedText = ref('')
const baseHash = ref('')
const revision = ref(0)
const busy = ref(false)
const dirty = computed(() => selected.value !== '' && text.value !== savedText.value)
const detail = ref<'editor' | 'history'>('editor')
const versions = ref<FileVersion[]>([])
const view = ref<'files' | 'mapping'>('files')

const current = computed(() => files.value.find((file) => file.path === selected.value))

type Row = { kind: 'folder'; path: string; name: string; depth: number } | { kind: 'file'; path: string; name: string; depth: number; file: FileInfo }
// Folders and files in path order; a collapsed folder hides what is inside it.
const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const seen = new Set<string>()
  for (const folder of fileFolders) {
    out.push({ kind: 'folder', path: folder, name: folder, depth: 0 })
    seen.add(folder)
  }
  for (const file of files.value) {
    const parts = file.path.split('/')
    for (let i = 1; i < parts.length; i++) {
      const folder = parts.slice(0, i).join('/')
      if (!seen.has(folder)) {
        seen.add(folder)
        out.push({ kind: 'folder', path: folder, name: parts[i - 1] ?? folder, depth: i - 1 })
      }
    }
    out.push({ kind: 'file', path: file.path, name: parts[parts.length - 1] ?? file.path, depth: parts.length - 1, file })
  }
  out.sort((a, b) => sortKey(a).localeCompare(sortKey(b)))
  return out.filter((row) => ![...collapsed.value].some((folder) => row.path.startsWith(`${folder}/`)))
})
// Folders sort before the files beside them.
function sortKey(row: Row): string {
  return row.kind === 'folder' ? `${row.path}/` : row.path.replace(/\/([^/]*)$/, '/~$1')
}

function describe(err: unknown, action: string): string {
  if (!(err instanceof ConnectError)) return `Could not ${action}.`
  if (err.code === Code.PermissionDenied) return `You do not have permission to ${action}.`
  if (err.code === Code.Aborted) return 'This file changed since you opened it. Reload it, then apply your edit again.'
  return `Could not ${action}: ${err.rawMessage}`
}

async function refresh() {
  try {
    const res = await fileClient.listFiles({})
    files.value = res.files
    withoutFile.value = res.recordTypesWithoutFile
    unavailable.value = ''
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
      unavailable.value = err.rawMessage.includes('Workspace directory')
        ? 'This server keeps no Workspace directory, so there are no files to show. Ask the person who runs Athena to start it with athena run.'
        : err.rawMessage
    }
    else toast.error(describe(err, 'load the files'))
  } finally {
    loaded.value = true
  }
}

// Each open gets a number so a slow response for a previously tapped file
// never overwrites the current editor.
let openRequest = 0

async function loadVersions() {
  const path = selected.value
  if (!path) return
  try {
    const res = await fileClient.listFileVersions({ path })
    if (selected.value === path) versions.value = res.versions
  } catch (err) {
    if (selected.value !== path) return
    versions.value = []
    if (!(err instanceof ConnectError && err.code === Code.NotFound)) toast.error(describe(err, 'load the history'))
  }
}

async function open(path: string) {
  const request = ++openRequest
  try {
    const res = await fileClient.getFile({ path })
    if (request !== openRequest) return
    text.value = decoder.decode(res.content)
    savedText.value = text.value
    baseHash.value = res.info?.hash ?? ''
    revision.value = Number(res.info?.revision ?? 0)
    selected.value = path
    detail.value = 'editor'
    versions.value = []
  } catch (err) {
    if (request !== openRequest) return
    toast.error(describe(err, 'open the file'))
  }
}

// Switching away from unsaved edits asks first.
const discardFor = ref('')
function choose(path: string) {
  if (path === selected.value) return
  if (dirty.value) discardFor.value = path
  else void open(path)
}

// The §16 detail sheet: tapping a tree row inspects the file (path, size,
// versions, backing Record Type or Rule) with Open in editor, Rename,
// Download and Delete. The editor workspace stays a page area.
const detailPath = ref('')
const detailVersions = ref<FileVersion[]>([])
const detailFile = computed(() => files.value.find((file) => file.path === detailPath.value) ?? null)
const detailName = computed(() => detailPath.value.split('/').pop() ?? detailPath.value)
let detailVersionsRequest = 0

async function loadDetailVersions() {
  const path = detailPath.value
  detailVersions.value = []
  if (!path) return
  const request = ++detailVersionsRequest
  try {
    const res = await fileClient.listFileVersions({ path })
    if (request === detailVersionsRequest) detailVersions.value = res.versions
  } catch {
    if (request === detailVersionsRequest) detailVersions.value = []
  }
}

watch(detailPath, () => { void loadDetailVersions() })

function openDetail(path: string) {
  detailPath.value = detailPath.value === path ? '' : path
}

function openInEditor() {
  const path = detailPath.value
  detailPath.value = ''
  if (path) choose(path)
}

function formatSize(bytes: bigint): string {
  const n = Number(bytes)
  if (n < 1024) return `${n} bytes`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`
  return `${(n / 1024 / 1024).toFixed(1)} MiB`
}

function backingLine(file: FileInfo): string {
  const b = file.backing
  if (!b) return 'No Record Type or Rule uses this file.'
  return `${kindLabels[b.kind] ?? b.kind} ${b.key} (${b.saved ? 'saved' : 'not saved yet'})`
}

const detailActions = computed<ActionItem[]>(() => {
  const items: ActionItem[] = [{ id: 'open', label: 'Open in editor', kind: 'primary', icon: 'pencil', busy: busy.value }]
  items.push({ id: 'download', label: 'Download', kind: 'secondary', icon: 'download', busy: busy.value })
  if (props.canWrite) {
    items.push({ id: 'rename', label: 'Rename', kind: 'secondary', icon: 'pencil', busy: busy.value })
    items.push({ id: 'delete', label: 'Delete', kind: 'destructive', icon: 'trash-2', busy: busy.value })
  }
  return items
})

function onDetailAction(id: string) {
  if (id === 'open') openInEditor()
  else if (id === 'download') void downloadDetail()
  else if (id === 'rename') startRename(detailPath.value)
  else if (id === 'delete') deleting.value = true
}
function discard() {
  const path = discardFor.value
  discardFor.value = ''
  void open(path)
}

async function save() {
  if (!props.canWrite || !dirty.value) return
  if (!withinFileLimit(text.value)) {
    toast.error(`The file is over the ${maxFileBytes / 1024 / 1024} MiB limit.`)
    return
  }
  busy.value = true
  try {
    const res = await fileClient.putFile({ path: selected.value, content: encoder.encode(text.value), expectedHash: baseHash.value })
    savedText.value = text.value
    baseHash.value = res.info?.hash ?? ''
    revision.value = Number(res.info?.revision ?? 0)
    toast.success(`Saved ${selected.value} (revision ${revision.value}).`)
    await refresh()
    if (detail.value === 'history') await loadVersions()
  } catch (err) {
    toast.error(describe(err, 'save the file'))
  } finally {
    busy.value = false
  }
}

async function reload() {
  if (selected.value) await open(selected.value)
}

function download() {
  const link = document.createElement('a')
  const url = URL.createObjectURL(new Blob([text.value], { type: 'text/plain;charset=utf-8' }))
  link.href = url
  link.download = selected.value.split('/').pop() ?? 'file.txt'
  link.click()
  URL.revokeObjectURL(url)
}

async function downloadDetail() {
  const path = detailPath.value
  if (!path) return
  try {
    const body = path === selected.value ? text.value : decoder.decode((await fileClient.getFile({ path })).content)
    const link = document.createElement('a')
    const url = URL.createObjectURL(new Blob([body], { type: 'text/plain;charset=utf-8' }))
    link.href = url
    link.download = path.split('/').pop() ?? 'file.txt'
    link.click()
    URL.revokeObjectURL(url)
  } catch (err) {
    toast.error(describe(err, 'download the file'))
  }
}

// New file, upload and rename share one dialog.
const dialog = ref<'' | 'new' | 'rename'>('')
const dialogPath = ref('')
const uploaded = ref<string | null>(null)
const pathProblem = computed(() => (dialogPath.value ? validateFilePath(dialogPath.value) : null))
const pathSnapshot = ref('')
watch(dialog, (open) => {
  if (open !== '') pathSnapshot.value = dialogPath.value
})
const pathDirty = computed(() => dialog.value !== '' && dialogPath.value !== pathSnapshot.value)

function startNew() {
  dialog.value = 'new'
  dialogPath.value = 'record_types/'
  uploaded.value = null
}
const renameFrom = ref('')
function startRename(path?: string) {
  if (dirty.value) {
    toast.error('Save or discard your edits before renaming.')
    return
  }
  renameFrom.value = path ?? selected.value
  dialog.value = 'rename'
  dialogPath.value = renameFrom.value
}
const picker = ref<HTMLInputElement | null>(null)
async function chosenUpload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (file.size > maxFileBytes) {
    toast.error(`${file.name} is over the ${maxFileBytes / 1024 / 1024} MiB limit.`)
    return
  }
  try {
    uploaded.value = decoder.decode(await file.arrayBuffer())
  } catch {
    toast.error(`${file.name} is not UTF-8 text, so it cannot be uploaded.`)
    return
  }
  dialog.value = 'new'
  dialogPath.value = `${selected.value.split('/')[0] ?? 'rules'}/${file.name.toLowerCase().replace(/[^a-z0-9_.-]+/g, '_')}`
}

// A new Record Type file starts from the smallest valid definition.
function template(path: string): string {
  const parts = path.split('/')
  if (parts[parts.length - 2] !== 'record_types' || !/\.ya?ml$/.test(path)) return ''
  const key = (parts[parts.length - 1] ?? '').replace(/\.ya?ml$/, '')
  return `api: athena/v1\nrecord_type: ${key}\nlabel: ${key}\nkind: master\nfields:\n  - name: title\n    type: Text\n`
}

async function submitDialog() {
  const path = dialogPath.value.trim()
  if (validateFilePath(path)) return
  busy.value = true
  try {
    if (dialog.value === 'new') {
      const body = uploaded.value ?? template(path)
      await fileClient.putFile({ path, content: encoder.encode(body), expectedHash: '' })
      toast.success(`Created ${path}.`)
      dialog.value = ''
      await refresh()
      await open(path)
    } else {
      const from = renameFrom.value || selected.value
      const hash = from === selected.value ? baseHash.value : (files.value.find((file) => file.path === from)?.hash ?? '')
      await fileClient.moveFile({ from, to: path, expectedHash: hash })
      toast.success(`Moved ${from} to ${path}.`)
      dialog.value = ''
      if (detailPath.value === from) detailPath.value = ''
      await refresh()
      await open(path)
    }
  } catch (err) {
    toast.error(describe(err, dialog.value === 'new' ? 'create the file' : 'move the file'))
  } finally {
    busy.value = false
  }
}

const deleting = ref(false)
// The editor sits behind the sheet overlay, so a Delete press with the sheet
// open always means the sheet file.
const deleteTarget = computed(() => detailPath.value || selected.value)
async function remove() {
  deleting.value = false
  const path = deleteTarget.value
  if (!path) return
  busy.value = true
  try {
    const hash = path === selected.value ? baseHash.value : (files.value.find((file) => file.path === path)?.hash ?? '')
    await fileClient.deleteFile({ path, expectedHash: hash })
    toast.success(`Deleted ${path}. Its versions are kept.`)
    if (path === selected.value) {
      selected.value = ''
      text.value = savedText.value = ''
    }
    if (path === detailPath.value) detailPath.value = ''
    await refresh()
  } catch (err) {
    toast.error(describe(err, 'delete the file'))
  } finally {
    busy.value = false
  }
}

async function loadRevision(version: FileVersion) {
  try {
    const res = await fileClient.getFile({ path: version.path, revision: version.revision })
    text.value = decoder.decode(res.content)
    detail.value = 'editor'
    toast.success(`Revision ${version.revision} is in the editor. Save to make it the current file.`)
  } catch (err) {
    toast.error(describe(err, 'load that revision'))
  }
}

function onDetail(value: string | number) {
  detail.value = value === 'history' ? 'history' : 'editor'
  if (detail.value === 'history') void loadVersions()
}

const backed = computed(() => files.value.filter((file) => file.backing))
function backingStatus(file: FileInfo): string {
  const b = file.backing
  if (!b) return ''
  if (b.saved) return 'Saved Record Type'
  if (b.kind === 'record_type' || b.kind === 'generated_source') return 'No saved Record Type'
  return 'Not implemented yet'
}
const kindLabels: Record<string, string> = { record_type: 'Record Type', rule: 'Rule', report: 'Report', generated_source: 'Generated source' }

onMounted(refresh)
</script>

<template>
  <section class="page-content files-page">
    <div class="page-title-row">
      <div><h1>Files</h1><p class="lead">Source files for Record Types, Rules and Packs.</p></div>
      <div v-if="canWrite && !unavailable" class="toolbar-actions">
        <ActionButton kind="secondary" label="Upload" icon="upload" @press="picker?.click()" />
        <ActionButton kind="primary" label="New file" icon="plus" @press="startNew" />
        <input ref="picker" class="files-picker" type="file" :accept="fileExtensions.join(',')" aria-label="Upload a file" @change="chosenUpload">
      </div>
    </div>

    <div v-if="unavailable" class="empty-builder"><div class="card-icon">▤</div><h2>No Workspace directory</h2><p>{{ unavailable }}</p></div>
    <template v-else>
      <Tabs v-model="view" class="files-tabs">
        <TabsList>
          <TabsTrigger value="files">Files</TabsTrigger>
          <TabsTrigger value="mapping">Mapping</TabsTrigger>
        </TabsList>

        <TabsContent value="files">
          <div class="files-layout">
            <nav class="files-tree" aria-label="Workspace files">
              <p v-if="loaded && files.length === 0" class="files-empty">No files yet. {{ canWrite ? 'Create one or upload one.' : '' }}</p>
              <template v-for="row in rows" :key="row.path">
                <button v-if="row.kind === 'folder'" class="files-node is-folder" :style="{ paddingLeft: `${8 + row.depth * 14}px` }" @click="collapsed.has(row.path) ? collapsed.delete(row.path) : collapsed.add(row.path)">
                  <span aria-hidden="true">{{ collapsed.has(row.path) ? '▸' : '▾' }}</span>{{ row.name }}
                </button>
                <button v-else class="files-node" :class="{ chosen: row.path === detailPath || row.path === selected }" :style="{ paddingLeft: `${8 + row.depth * 14}px` }" :title="row.path" @click="openDetail(row.path)">
                  <span class="files-name">{{ row.name }}</span>
                  <span v-if="!row.file.recorded && row.file.revision > 0" class="files-flag" title="Changed outside Athena">edited</span>
                </button>
              </template>
            </nav>

            <div class="record-editor files-editor-card">
              <p v-if="!selected" class="files-empty">Select a file to read it.</p>
              <template v-else>
                <div class="record-editor-head">
                  <h2 class="files-title">{{ selected }}</h2>
                  <Badge variant="secondary">r{{ revision }}</Badge>
                  <Badge v-if="dirty" variant="outline">Unsaved</Badge>
                  <div class="toolbar-actions">
                    <ActionButton v-if="canWrite" kind="primary" label="Save" icon="save" :busy="busy" :disabled-reason="!dirty ? 'No changes to save.' : null" @press="save" />
                    <ActionButton kind="secondary" label="Download" icon="download" @press="download" />
                    <ActionButton v-if="canWrite" kind="secondary" label="Rename" icon="pencil" :busy="busy" @press="startRename" />
                    <ActionButton v-if="canWrite" kind="destructive" label="Delete" icon="trash-2" :busy="busy" @press="deleting = true" />
                  </div>
                </div>
                <Tabs :model-value="detail" @update:model-value="onDetail">
                  <TabsList>
                    <TabsTrigger value="editor">Editor</TabsTrigger>
                    <TabsTrigger value="history">History</TabsTrigger>
                  </TabsList>
                  <TabsContent value="editor">
                    <Textarea v-model="text" class="files-editor" spellcheck="false" :readonly="!canWrite" :aria-label="`Contents of ${selected}`" @keydown.meta.s.prevent="save" @keydown.ctrl.s.prevent="save" />
                    <p class="files-hint">{{ current?.backing ? `${kindLabels[current.backing.kind] ?? current.backing.kind}: ${current.backing.key}` : 'Not tied to a Record Type, Rule or Report.' }}
                      <ActionButton v-if="dirty" kind="quiet" density="inline" label="Discard edits" icon="undo-2" @press="reload" />
                      <ActionButton v-else kind="quiet" density="inline" label="Reload from disk" icon="refresh-ccw" @press="reload" /></p>
                  </TabsContent>
                  <TabsContent value="history">
                    <Table class="files-table">
                      <TableHeader><TableRow><TableHead>Revision</TableHead><TableHead>Change</TableHead><TableHead>When</TableHead><TableHead>By</TableHead><TableHead>Hash</TableHead><TableHead /></TableRow></TableHeader>
                      <TableBody>
                        <TableRow v-for="version in versions" :key="String(version.revision)">
                          <TableCell>r{{ version.revision }}</TableCell>
                          <TableCell>{{ version.action }}<small v-if="version.path !== selected" class="files-sub"> {{ version.path }}</small></TableCell>
                          <TableCell>{{ formatWhen(version.createdAt) }}</TableCell>
                          <TableCell>{{ version.actor }}</TableCell>
                          <TableCell><code>{{ version.hash.slice(0, 10) }}</code></TableCell>
                          <TableCell><ActionButton v-if="version.hasContent && canWrite" kind="restore" density="row" label="Load into editor" icon="rotate-ccw" :busy="busy" @press="loadRevision(version)" /></TableCell>
                        </TableRow>
                      </TableBody>
                    </Table>
                    <p v-if="versions.length === 0" class="files-hint">No recorded versions.</p>
                  </TabsContent>
                </Tabs>
              </template>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="mapping">
          <div class="record-editor">
            <p class="files-hint">Which file backs which Record Type, Rule or Report, taken from the folder and file name. Rules and Reports are not implemented yet, so they have no saved definition.</p>
            <Table class="files-table">
              <TableHeader><TableRow><TableHead>File</TableHead><TableHead>Backs</TableHead><TableHead>Key</TableHead><TableHead>Status</TableHead></TableRow></TableHeader>
              <TableBody>
                <TableRow v-for="file in backed" :key="file.path" class="files-map-row" tabindex="0" @click="detailPath = file.path; view = 'files'" @keydown.enter="detailPath = file.path; view = 'files'">
                  <TableCell>{{ file.path }}</TableCell>
                  <TableCell>{{ kindLabels[file.backing?.kind ?? ''] }}</TableCell>
                  <TableCell><code>{{ file.backing?.key }}</code></TableCell>
                  <TableCell>{{ backingStatus(file) }}</TableCell>
                </TableRow>
                <TableRow v-for="name in withoutFile" :key="name">
                  <TableCell class="files-sub">No file</TableCell>
                  <TableCell>Record Type</TableCell>
                  <TableCell><code>{{ name }}</code></TableCell>
                  <TableCell>Saved, but no file under record_types/</TableCell>
                </TableRow>
              </TableBody>
            </Table>
            <p v-if="backed.length === 0 && withoutFile.length === 0" class="files-hint">Nothing to map yet.</p>
          </div>
        </TabsContent>
      </Tabs>
    </template>

    <ResponsiveDialog
      :open="dialog !== ''"
      :title="dialog === 'rename' ? 'Rename file' : uploaded !== null ? 'Upload file' : 'New file'"
      :description="'Folders: ' + fileFolders.join(', ') + '. Lowercase letters, digits, _, - and . only. Types: ' + fileExtensions.join(' ') + '.'"
      :dirty="pathDirty"
      @update:open="(value: boolean) => { if (!value) dialog = '' }"
    >
        <form id="file-path" class="sign-in-form" @submit.prevent="submitDialog">
          <TextField id="file-path-input" label="Path" v-model="dialogPath" autocomplete="off" :error="pathProblem ?? undefined" />
        </form>
        <template #footer>
          <ActionButton kind="secondary" label="Cancel" icon="x" @press="dialog = ''" />
          <ActionButton type="submit" form="file-path" kind="primary" :label="dialog === 'rename' ? 'Rename' : 'Create'" :icon="dialog === 'rename' ? 'pencil' : 'plus'" :busy="busy" :disabled-reason="!dialogPath ? 'Enter a path.' : pathProblem" />
        </template>
    </ResponsiveDialog>

    <ConfirmDialog
      v-model:open="deleting"
      :title="`Delete ${deleteTarget}?`"
      body="The file is removed from disk. Its versions stay in the history."
      confirm-label="Delete file"
      cancel-label="Keep it"
      :busy="busy"
      @confirm="remove"
    />

    <ConfirmDialog
      :open="discardFor !== ''"
      title="Discard your edits?"
      :body="`${selected} has changes that are not saved.`"
      confirm-label="Discard edits"
      cancel-label="Keep editing"
      confirm-icon="x"
      @confirm="discard"
      @update:open="(value: boolean) => { if (!value) discardFor = '' }"
    />

    <DetailSheet
      v-if="detailFile"
      :open="true"
      :title="detailName"
      @update:open="(value: boolean) => { if (!value) detailPath = '' }"
    >
      <template #subtitle>
        <p class="text-caption text-muted-foreground font-mono">{{ detailFile.path }}</p>
      </template>
      <template #status>
        <Badge variant="secondary">r{{ detailFile.revision }}</Badge>
        <Badge v-if="!detailFile.recorded && detailFile.revision > BigInt(0)" variant="outline">Edited outside Athena</Badge>
      </template>
      <div class="sheet-section">
        <p class="sheet-label">Size</p>
        <p class="sheet-lead">{{ formatSize(detailFile.size) }}</p>
        <p v-if="detailFile.modifiedAt" class="muted-label">Changed {{ formatWhen(detailFile.modifiedAt) }}{{ detailFile.modifiedBy ? ` by ${detailFile.modifiedBy}` : '' }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Backing</p>
        <p class="sheet-lead">{{ backingLine(detailFile) }}</p>
      </div>
      <div class="sheet-section">
        <p class="sheet-label">Versions</p>
        <ul v-if="detailVersions.length > 0" class="sheet-events">
          <li v-for="version in detailVersions" :key="String(version.revision)"><span>r{{ version.revision }} · {{ version.action }}</span><span class="muted-label">{{ formatWhen(version.createdAt) }} · {{ version.actor }}</span></li>
        </ul>
        <p v-else class="files-empty">No recorded versions.</p>
      </div>
      <template #footer>
        <ActionGroup :actions="detailActions" stack-on-phones aria-label="File actions" @press="onDetailAction" />
      </template>
    </DetailSheet>
  </section>
</template>
