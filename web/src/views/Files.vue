<template>
  <div class="wm-page">
    <WmSectionCard title="文件浏览" :description="path" flush>
      <WmToolbar>
        <a-input v-model:value="pathInput" class="wm-path-input" placeholder="输入绝对路径" @keyup.enter="go" />
        <a-button @click="go">跳转</a-button>
        <a-button @click="goUp">上级</a-button>
        <a-button @click="load">刷新</a-button>
        <span class="wm-toolbar__sep" />
        <a-button type="primary" @click="mkdir">新建目录</a-button>
        <a-upload :custom-request="doUpload" :show-upload-list="false">
          <a-button data-tone="success">上传文件</a-button>
        </a-upload>
        <a-dropdown :disabled="!selection.length" trigger="['click']">
          <a-button :disabled="!selection.length">压缩选中</a-button>
          <template #overlay>
            <a-menu @click="({ key }: any) => compress(String(key))">
              <a-menu-item key="zip">压缩为 .zip</a-menu-item>
              <a-menu-item key="targz">压缩为 .tar.gz</a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
        <a-button data-tone="danger" :disabled="!selection.length" @click="delSelected">删除选中</a-button>
        <template #note v-if="selection.length">已选中 {{ selection.length }} 项，可使用批量操作。</template>
      </WmToolbar>

      <WmTable :data="items" v-loading="loading" size="small" @selection-change="(v: any) => (selection = v)" empty-title="此目录为空" empty-description="可上传文件、新建目录，或在上方输入路径跳转。">
        <WmColumn type="selection" width="42" />
        <WmColumn label="名称" min-width="300">
          <template #default="{ row }">
            <a v-if="row.is_dir" @click="enter(row)" class="wm-link"><span class="wm-icon"><Folder /></span>&nbsp;{{ row.name }}</a>
            <span v-else>{{ row.name }}</span>
          </template>
        </WmColumn>
        <WmColumn label="大小" width="110"><template #default="{ row }"><span class="wm-mono">{{ row.is_dir ? '-' : fmtBytes(row.size) }}</span></template></WmColumn>
        <WmColumn prop="mode" label="权限" width="110"><template #default="{ row }"><span class="wm-mono">{{ row.mode }}</span></template></WmColumn>
        <WmColumn prop="owner" label="属主" width="110" />
        <WmColumn label="修改时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.mtime) }}</span></template></WmColumn>
        <WmColumn label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <a-button v-if="row.is_dir" size="small" @click="enter(row)">打开</a-button>
            <a-button v-if="!row.is_dir && row.size <= 2 * 1024 * 1024" size="small" @click="edit(row)">编辑</a-button>
            <a-button v-if="!row.is_dir" size="small" @click="download(row)">下载</a-button>
            <a-button v-if="isArchive(row.name)" size="small" data-tone="success" @click="decompress(row)">解压</a-button>
            <a-button size="small" @click="rename(row)">重命名</a-button>
            <a-button size="small" data-tone="danger" @click="delOne(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-modal v-model:open="editDlg" :title="`编辑 - ${editPath}`" width="760px">
      <a-textarea v-model:value="editContent"  :rows="18" />
      <template #footer>
        <a-button @click="editDlg = false">取消</a-button>
        <a-button type="primary" @click="saveEdit">保存</a-button>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import { Folder } from '../antd-compat'
import api from '../api'
import { fmtBytes, fmtTime } from '../utils'

const path = ref('/')
const pathInput = ref('/')
const items = ref<any[]>([])
const loading = ref(false)
const selection = ref<any[]>([])

function isArchive(name: string) {
  const n = name.toLowerCase()
  return n.endsWith('.zip') || n.endsWith('.tar.gz') || n.endsWith('.tgz')
}

function parentPath(p: string) {
  p = p.replace(/[\\/]+$/, '')
  const i = Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/'))
  if (i <= 0) return /^[A-Za-z]:/.test(p) ? p : '/'
  let np = p.slice(0, i)
  if (/^[A-Za-z]:$/.test(np)) np += '/'
  if (np === '') np = '/'
  return np
}

async function load() {
  loading.value = true
  try {
    const d: any = await api.get('/files/list', { params: { path: path.value } })
    path.value = d.path
    pathInput.value = d.path
    items.value = d.items
  } finally {
    loading.value = false
  }
}

function enter(row: any) {
  path.value = row.path
  load()
}

function go() {
  path.value = pathInput.value
  load()
}

function goUp() {
  path.value = parentPath(path.value)
  load()
}

async function ask(title: string, msg: string, ph = ''): Promise<string | null> {
  try {
    const r: any = await ElMessageBox.prompt(msg, title, { inputValue: ph })
    return r.value
  } catch {
    return null
  }
}

async function ok(msg: string): Promise<boolean> {
  try {
    await ElMessageBox.confirm(msg, '确认', { type: 'warning' })
    return true
  } catch {
    return false
  }
}

async function mkdir() {
  const v = await ask('新建目录', '输入目录名称')
  if (!v) return
  await api.post('/files/mkdir', { path: path.value, name: v })
  load()
}

async function rename(row: any) {
  const v = await ask('重命名', '输入新名称', row.name)
  if (!v || v === row.name) return
  await api.post('/files/rename', { path: row.path, name: v })
  load()
}

async function delOne(row: any) {
  if (!(await ok(`确认删除「${row.name}」？将移入面板回收站`))) return
  await api.post('/files/delete', { paths: [row.path] })
  load()
}

async function delSelected() {
  if (!(await ok(`确认删除选中的 ${selection.value.length} 项？将移入面板回收站`))) return
  await api.post('/files/delete', { paths: selection.value.map((s: any) => s.path) })
  load()
}

async function compress(fmt: string) {
  const r: any = await api.post('/files/compress', { paths: selection.value.map((s: any) => s.path), format: fmt })
  ElMessage.success('已生成: ' + r.path)
  load()
}

async function decompress(row: any) {
  const r: any = await api.post('/files/decompress', { path: row.path })
  ElMessage.success('已解压到: ' + r.path)
  load()
}

const editDlg = ref(false)
const editPath = ref('')
const editContent = ref('')

async function edit(row: any) {
  const d: any = await api.get('/files/read', { params: { path: row.path } })
  editPath.value = row.path
  editContent.value = d.content
  editDlg.value = true
}

async function saveEdit() {
  await api.post('/files/save', { path: editPath.value, content: editContent.value })
  ElMessage.success('已保存')
  editDlg.value = false
  load()
}

function download(row: any) {
  const a = document.createElement('a')
  a.href = `/api/files/download?path=${encodeURIComponent(row.path)}`
  a.click()
}

async function doUpload(opt: any) {
  const file: File = opt.file
  const CHUNK = 5 * 1024 * 1024
  if (file.size <= CHUNK) {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('path', path.value)
    await api.post('/files/upload', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    ElMessage.success('上传完成')
    load()
    return
  }
  // 大文件分片上传（5MB/片，body 直传原始字节）
  const uploadId = (crypto.randomUUID?.() || Math.random().toString(16).slice(2)).replace(/-/g, '').padEnd(32, '0').slice(0, 32)
  const total = Math.ceil(file.size / CHUNK)
  const target = path.value.replace(/\/+$/, '') + '/' + file.name
  for (let i = 0; i < total; i++) {
    const blob = file.slice(i * CHUNK, (i + 1) * CHUNK)
    await api.post(
      `/files/upload-chunk?path=${encodeURIComponent(target)}&upload_id=${uploadId}&index=${i}`,
      blob,
      { headers: { 'Content-Type': 'application/octet-stream' }, timeout: 600000 }
    )
    ElMessage.success(`上传中 ${i + 1}/${total} 片`)
  }
  const d: any = await api.post('/files/upload-finish', { path: target, upload_id: uploadId, total }, { timeout: 300000 })
  ElMessage.success('上传完成（' + ((d.size || 0) / 1048576).toFixed(1) + ' MB）')
  load()
}


onMounted(load)
</script>
