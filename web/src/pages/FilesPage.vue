<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Code, ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { fileClient } from '../api'
import type { FileInfo, FileVersion } from '../gen/athena/v1/files_pb'
import { fileExtensions, fileFolders, maxFileBytes, validateFilePath, withinFileLimit } from '../filePaths'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'

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
    if (err instanceof ConnectError && err.code === Code.FailedPrecondition) unavailable.value = err.rawMessage
    else toast.error(describe(err, 'load the files'))
  } finally {
    loaded.value = true
  }
}

async function loadVersions() {
  if (!selected.value) return
  try {
    versions.value = (await fileClient.listFileVersions({ path: selected.value })).versions
  } catch (err) {
    versions.value = []
    if (!(err instanceof ConnectError && err.code === Code.NotFound)) toast.error(describe(err, 'load the history'))
  }
}

async function open(path: string) {
  try {
    const res = await fileClient.getFile({ path })
    text.value = decoder.decode(res.content)
    savedText.value = text.value
    baseHash.value = res.info?.hash ?? ''
    revision.value = Number(res.info?.revision ?? 0)
    selected.value = path
    detail.value = 'editor'
    versions.value = []
  } catch (err) {
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

// New file, upload and rename share one dialog.
const dialog = ref<'' | 'new' | 'rename'>('')
const dialogPath = ref('')
const uploaded = ref<string | null>(null)
const pathProblem = computed(() => (dialogPath.value ? validateFilePath(dialogPath.value) : null))

function startNew() {
  dialog.value = 'new'
  dialogPath.value = 'record_types/'
  uploaded.value = null
}
function startRename() {
  if (dirty.value) {
    toast.error('Save or discard your edits before renaming.')
    return
  }
  dialog.value = 'rename'
  dialogPath.value = selected.value
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
      const from = selected.value
      await fileClient.moveFile({ from, to: path, expectedHash: baseHash.value })
      toast.success(`Moved ${from} to ${path}.`)
      dialog.value = ''
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
async function remove() {
  deleting.value = false
  busy.value = true
  try {
    await fileClient.deleteFile({ path: selected.value, expectedHash: baseHash.value })
    toast.success(`Deleted ${selected.value}. Its versions are kept.`)
    selected.value = ''
    text.value = savedText.value = ''
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

function when(iso: string): string {
  return iso ? new Date(iso).toLocaleString() : ''
}

onMounted(refresh)
</script>

<template>
  <section class="page-content files-page">
    <div class="page-title-row">
      <div><h1>Files</h1><p class="lead">Record Type definitions, Rule sources, Pack layout and generated source in this Workspace.</p></div>
      <div v-if="canWrite && !unavailable" class="toolbar-actions">
        <Button variant="outline" @click="picker?.click()">Upload</Button>
        <Button @click="startNew">＋ New file</Button>
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
                <button v-else class="files-node" :class="{ chosen: row.path === selected }" :style="{ paddingLeft: `${8 + row.depth * 14}px` }" :title="row.path" @click="choose(row.path)">
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
                    <Button v-if="canWrite" :disabled="!dirty || busy" @click="save">Save</Button>
                    <Button variant="outline" @click="download">Download</Button>
                    <Button v-if="canWrite" variant="outline" :disabled="busy" @click="startRename">Rename</Button>
                    <Button v-if="canWrite" variant="ghost" :disabled="busy" @click="deleting = true">Delete</Button>
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
                      <Button v-if="dirty" variant="link" size="sm" @click="reload">Discard edits</Button>
                      <Button v-else variant="link" size="sm" @click="reload">Reload from disk</Button></p>
                  </TabsContent>
                  <TabsContent value="history">
                    <Table class="files-table">
                      <TableHeader><TableRow><TableHead>Revision</TableHead><TableHead>Change</TableHead><TableHead>When</TableHead><TableHead>By</TableHead><TableHead>Hash</TableHead><TableHead /></TableRow></TableHeader>
                      <TableBody>
                        <TableRow v-for="version in versions" :key="String(version.revision)">
                          <TableCell>r{{ version.revision }}</TableCell>
                          <TableCell>{{ version.action }}<small v-if="version.path !== selected" class="files-sub"> {{ version.path }}</small></TableCell>
                          <TableCell>{{ when(version.createdAt) }}</TableCell>
                          <TableCell>{{ version.actor }}</TableCell>
                          <TableCell><code>{{ version.hash.slice(0, 10) }}</code></TableCell>
                          <TableCell><Button v-if="version.hasContent && canWrite" variant="ghost" size="sm" @click="loadRevision(version)">Load into editor</Button></TableCell>
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
                <TableRow v-for="file in backed" :key="file.path" class="files-map-row" tabindex="0" @click="open(file.path); view = 'files'" @keydown.enter="open(file.path); view = 'files'">
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

    <Dialog :open="dialog !== ''" @update:open="(value: boolean) => { if (!value) dialog = '' }">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ dialog === 'rename' ? 'Rename file' : uploaded !== null ? 'Upload file' : 'New file' }}</DialogTitle>
          <DialogDescription>Folders: {{ fileFolders.join(', ') }}. Lowercase letters, digits, _, - and . only. Types: {{ fileExtensions.join(' ') }}.</DialogDescription>
        </DialogHeader>
        <form id="file-path" class="sign-in-form" @submit.prevent="submitDialog">
          <div class="settings-field"><Label for="file-path-input">Path</Label><Input id="file-path-input" v-model="dialogPath" autocomplete="off" spellcheck="false" :aria-invalid="pathProblem !== null" /><small v-if="pathProblem" class="files-problem">{{ pathProblem }}</small></div>
        </form>
        <DialogFooter>
          <Button variant="outline" @click="dialog = ''">Cancel</Button>
          <Button type="submit" form="file-path" :disabled="busy || !dialogPath || pathProblem !== null">{{ dialog === 'rename' ? 'Rename' : 'Create' }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <AlertDialog :open="deleting" @update:open="(value: boolean) => { deleting = value }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Delete {{ selected }}?</AlertDialogTitle><AlertDialogDescription>The file is removed from disk. Its versions stay in the history.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep it</AlertDialogCancel><AlertDialogAction variant="destructive" @click="remove">Delete file</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <AlertDialog :open="discardFor !== ''" @update:open="(value: boolean) => { if (!value) discardFor = '' }">
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>Discard your edits?</AlertDialogTitle><AlertDialogDescription>{{ selected }} has changes that are not saved.</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction variant="destructive" @click="discard">Discard edits</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </section>
</template>
