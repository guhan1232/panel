<template>
  <div class="wm-page">
    <WmSectionCard title="数据库连接" description="连接凭据由面板托管，同时供计划任务的数据库备份使用">
      <a-segmented v-model:value="engine" class="wm-mb-4" :options="[
        { value: 'mysql', label: 'MySQL' },
        { value: 'pg', label: 'PostgreSQL' },
        { value: 'backup', label: '备份管理' }
      ]" />
      <template v-if="engine === 'mysql'">
          <WmNotice v-if="mysqlMissing" tone="warning" title="MySQL/MariaDB 服务未运行">
            探测 {{ mysql.host }}:{{ mysql.port }} 无监听（服务未安装或未启动），数据库管理功能暂不可用。
            <div class="wm-mt-4">
              <a-button type="primary" :loading="installingDb === 'mysql'" @click="installDb('mysql')">一键安装 MySQL（推荐）</a-button>
              <a-button :loading="installingDb === 'mariadb'" @click="installDb('mariadb')" style="margin-left:12px">安装 MariaDB（备选）</a-button>
              <p class="wm-text-muted" style="margin-top:8px">MySQL 优先（Ubuntu 走官方源；Debian 自动添加 MySQL 官方 APT 仓库），失败可改用 MariaDB。安装后自动修复 root 免密认证（Error 1698）并保存连接配置。</p>
            </div>
          </WmNotice>
          <template v-else>
          <a-form layout="inline" class="cfg">
            <a-form-item label="主机"><a-input v-model:value="mysql.host" style="width: 130px" /></a-form-item>
            <a-form-item label="端口"><a-input-number v-model:value="mysql.port" :min="1" controls-position="right" style="width: 110px" /></a-form-item>
            <a-form-item label="用户"><a-input v-model:value="mysql.user" style="width: 120px" /></a-form-item>
            <a-form-item label="密码"><a-input-password v-model:value="mysql.password"  style="width: 140px" /></a-form-item>
            <a-button @click="saveCfg('mysql')">保存配置</a-button>
            <a-button type="primary" @click="testConn('mysql')">测试连接</a-button>
          </a-form>
          <WmToolbar class="wm-mt-4">
            <a-button size="small" @click="createUser('mysql')">创建用户</a-button>
            <a-button size="small" @click="grant('mysql')">授权用户到库</a-button>
            <template #end>
              <a-dropdown>
                <a-button size="small">更多<span class="wm-icon wm-dd-icon"><ArrowDown /></span></a-button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item key="uninstall" :disabled="installingDb === 'uninstall'" @click="uninstallDb('mysql')">卸载 MySQL 服务</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
              <a-button size="small" type="primary" @click="createDb('mysql')">创建数据库</a-button>
              <a-button size="small" @click="loadData">刷新</a-button>
            </template>
          </WmToolbar>
          <div class="wm-grid-2">
            <WmSectionCard title="数据库" dense quiet>
              <template #actions><span class="wm-text-muted wm-row-stats">{{ mysqlData.dbs.length }} 个库</span></template>
              <WmTable :data="mysqlData.dbs" size="small" max-height="360" empty-title="暂无数据库">
                <WmColumn label="库名" prop="name" min-width="160"><template #default="{ row }"><span class="wm-mono">{{ row.name }}</span></template></WmColumn>
                <WmColumn label="操作" width="72">
                  <template #default="{ row }"><a-button type="link" size="small" danger @click="dropDb('mysql', row.name)">删除</a-button></template>
                </WmColumn>
              </WmTable>
            </WmSectionCard>
            <WmSectionCard title="用户" dense quiet>
              <template #actions><span class="wm-text-muted wm-row-stats">{{ mysqlData.users.length }} 个用户</span></template>
              <WmTable :data="mysqlData.users" size="small" max-height="360" empty-title="暂无用户">
                <WmColumn label="用户" prop="user" min-width="110"><template #default="{ row }"><span class="wm-mono">{{ row.user }}</span></template></WmColumn>
                <WmColumn label="主机" prop="host" min-width="110"><template #default="{ row }"><span class="wm-mono">{{ row.host }}</span></template></WmColumn>
                <WmColumn label="操作" width="72">
                  <template #default="{ row }"><a-button type="link" size="small" danger @click="dropUser('mysql', row)">删除</a-button></template>
                </WmColumn>
              </WmTable>
            </WmSectionCard>
          </div>
          </template>
      </template>
      <template v-else-if="engine === 'pg'">
          <WmNotice v-if="pgMissing" tone="warning" title="PostgreSQL 服务未运行">
            探测 {{ pg.host }}:{{ pg.port }} 无监听（服务未安装或未启动）。可通过「软件商店」或系统包管理器安装 PostgreSQL 后，在此保存连接配置。
          </WmNotice>
          <template v-else>
          <a-form layout="inline" class="cfg">
            <a-form-item label="主机"><a-input v-model:value="pg.host" style="width: 130px" /></a-form-item>
            <a-form-item label="端口"><a-input-number v-model:value="pg.port" :min="1" controls-position="right" style="width: 110px" /></a-form-item>
            <a-form-item label="用户"><a-input v-model:value="pg.user" style="width: 120px" /></a-form-item>
            <a-form-item label="密码"><a-input-password v-model:value="pg.password"  style="width: 140px" /></a-form-item>
            <a-button @click="saveCfg('pg')">保存配置</a-button>
            <a-button type="primary" @click="testConn('pg')">测试连接</a-button>
          </a-form>
          <WmToolbar class="wm-mt-4">
            <a-button size="small" @click="createUser('pg')">创建用户</a-button>
            <a-button size="small" @click="grant('pg')">授权用户到库</a-button>
            <template #end>
              <a-dropdown>
                <a-button size="small">更多<span class="wm-icon wm-dd-icon"><ArrowDown /></span></a-button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item key="uninstall" :disabled="installingDb === 'uninstall'" @click="uninstallDb('postgresql')">卸载 PostgreSQL</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
              <a-button size="small" type="primary" @click="createDb('pg')">创建数据库</a-button>
              <a-button size="small" @click="loadData">刷新</a-button>
            </template>
          </WmToolbar>
          <div class="wm-grid-2">
            <WmSectionCard title="数据库" dense quiet>
              <template #actions><span class="wm-text-muted wm-row-stats">{{ pgData.dbs.length }} 个库</span></template>
              <WmTable :data="pgData.dbs" size="small" max-height="360" empty-title="暂无数据库">
                <WmColumn label="库名" min-width="160"><template #default="{ row }"><span class="wm-mono">{{ row }}</span></template></WmColumn>
                <WmColumn label="操作" width="72">
                  <template #default="{ row }"><a-button type="link" size="small" danger @click="dropDb('pg', row)">删除</a-button></template>
                </WmColumn>
              </WmTable>
            </WmSectionCard>
            <WmSectionCard title="角色" dense quiet>
              <template #actions><span class="wm-text-muted wm-row-stats">{{ pgData.users.length }} 个角色</span></template>
              <WmTable :data="pgData.users" size="small" max-height="360" empty-title="暂无角色">
                <WmColumn label="角色" min-width="160"><template #default="{ row }"><span class="wm-mono">{{ row }}</span></template></WmColumn>
                <WmColumn label="操作" width="72">
                  <template #default="{ row }"><a-button type="link" size="small" danger @click="dropUser('pg', row)">删除</a-button></template>
                </WmColumn>
              </WmTable>
            </WmSectionCard>
          </div>
          </template>
      </template>
      <template v-else>
          <WmToolbar>
            <a-select v-model:value="backupDb" size="small" placeholder="选择要备份的数据库" style="width: 220px">
              <a-select-opt-group v-if="mysqlData.dbs.length" label="MySQL">
                <a-select-option v-for="d in mysqlData.dbs" :key="'m' + d.name" :value="'mysql:' + d.name">{{ d.name }}（MySQL）</a-select-option>
              </a-select-opt-group>
              <a-select-opt-group v-if="pgData.dbs.length" label="PostgreSQL">
                <a-select-option v-for="d in pgData.dbs" :key="'p' + d.name" :value="'pg:' + d.name">{{ d.name }}（PG）</a-select-option>
              </a-select-opt-group>
            </a-select>
            <template #end>
              <a-button size="small" type="primary" :loading="backupRunning" @click="runBackup">立即备份</a-button>
              <a-button size="small" @click="loadBackups">刷新列表</a-button>
            </template>
          </WmToolbar>
          <WmTable :data="backupFiles" v-loading="bkLoading" size="small" max-height="360" empty-title="暂无备份" empty-description="选择数据库后点击「立即备份」，dump 为 gzip 压缩 SQL。">
            <WmColumn label="文件" min-width="300"><template #default="{ row }"><span class="wm-mono">{{ row.file }}</span></template></WmColumn>
            <WmColumn label="大小" width="110"><template #default="{ row }">{{ fmtSize(row.size) }}</template></WmColumn>
            <WmColumn label="时间" width="170"><template #default="{ row }">{{ row.time }}</template></WmColumn>
            <WmColumn label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <a-button type="link" size="small" @click="downloadBackup(row)">下载</a-button>
                <a-button type="link" size="small" danger @click="deleteBackup(row)">删除</a-button>
              </template>
            </WmColumn>
          </WmTable>
          <WmNotice tone="neutral" class="wm-mt-4">备份文件存于服务器 data/backups/ 目录，dump 经 gzip 压缩；计划任务可定时自动备份。</WmNotice>
      </template>
    </WmSectionCard>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox, ArrowDown } from '../antd-compat'
import api from '../api'

const engine = ref('mysql')
const mysql = reactive<any>({ host: '127.0.0.1', port: 3306, user: 'root', password: '' })
const pg = reactive<any>({ host: '127.0.0.1', port: 5432, user: 'postgres', password: '', ssl_mode: 'disable' })
const mysqlData = reactive<any>({ dbs: [], users: [] })
const pgData = reactive<any>({ dbs: [], users: [] })

const backupDb = ref('')
const backupFiles = ref<any[]>([])
const backupRunning = ref(false)
const bkLoading = ref(false)

function fmtSize(n: number) {
  if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(1) + ' KB'
  return n + ' B'
}

async function runBackup() {
  const v = backupDb.value || ''
  const i = v.indexOf(':')
  if (i < 0) {
    ElMessage.warning('请先选择要备份的数据库')
    return
  }
  const engine = v.slice(0, i)
  const database = v.slice(i + 1)
  backupRunning.value = true
  try {
    const d: any = await api.post('/dbmgr/backup', { engine, database }, { timeout: 1200000 })
    ElMessage.success('备份完成：' + (d.file || ''))
    loadBackups()
  } finally {
    backupRunning.value = false
  }
}

async function loadBackups() {
  bkLoading.value = true
  try {
    backupFiles.value = (await api.get('/dbmgr/backup/list')) as any
  } finally {
    bkLoading.value = false
  }
}

function downloadBackup(row: any) {
  window.open('/api/dbmgr/backup/download?file=' + encodeURIComponent(row.file))
}

async function deleteBackup(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除备份「${row.file}」？`, '删除备份', { type: 'warning' })
  } catch {
    return
  }
  await api.delete('/dbmgr/backup', { params: { file: row.file } })
  ElMessage.success('已删除')
  loadBackups()
}

async function loadCfg() {
  const cfg: any = await api.get('/dbmgr/config')
  Object.assign(mysql, cfg.mysql, { password: '' })
  Object.assign(pg, cfg.pg, { password: '' })
}

async function saveCfg(e: string) {
  if (e === 'mysql') await api.post('/dbmgr/mysql-config', { ...mysql })
  else await api.post('/dbmgr/pg-config', { ...pg })
  ElMessage.success('已保存')
}

async function testConn(e: string) {
  await saveCfg(e)
  await api.post('/dbmgr/test', { engine: e })
  ElMessage.success('连接成功')
}

async function loadData() {
  if (engine.value === 'mysql') {
    if (!(dbStatus.value?.mysql?.installed)) {
      mysqlMissing.value = true
      return
    }
    mysqlMissing.value = false
    const d1: any = await api.get('/dbmgr/mysql/databases')
    if (d1 && typeof d1 === 'object' && d1.installed === false) {
      mysqlMissing.value = true
      return
    }
    mysqlData.dbs = d1
    mysqlData.users = (await api.get('/dbmgr/mysql/users')) as any
  } else {
    if (!(dbStatus.value?.pg?.installed)) {
      pgMissing.value = true
      return
    }
    pgMissing.value = false
    const d1: any = await api.get('/dbmgr/pg/databases')
    if (d1 && typeof d1 === 'object' && d1.installed === false) {
      pgMissing.value = true
      return
    }
    pgData.dbs = d1
    pgData.users = (await api.get('/dbmgr/pg/users')) as any
  }
}

// ---- 数据库服务状态与一键安装 ----
const dbStatus = ref<any>({})
const mysqlMissing = ref(false)
const pgMissing = ref(false)
const installingDb = ref<string | false>(false)

async function loadStatus() {
  try {
    dbStatus.value = await api.get('/dbmgr/status')
  } catch {
    dbStatus.value = {}
  }
}

async function installDb(kind: 'mysql' | 'mariadb') {
  installingDb.value = kind
  try {
    const r: any = await api.post('/dbmgr/install-' + kind, {}, { timeout: 1500000 })
    ElMessage.success(r?.message || '安装完成')
    await loadStatus()
    loadCfg()
    loadData()
  } finally {
    installingDb.value = false
  }
}

async function uninstallDb(engine: 'mysql' | 'postgresql') {
  const label = engine === 'mysql' ? 'MySQL/MariaDB' : 'PostgreSQL'
  try {
    await ElMessageBox.confirm('确认卸载 ' + label + ' 服务？数据库管理功能将不可用。', '卸载服务', { type: 'warning' })
  } catch { return }
  let removeData = false
  try {
    await ElMessageBox.confirm('是否同时删除数据目录（/var/lib 下全部库文件，不可恢复）？', '删除数据', { type: 'error', confirmButtonText: '删除数据', cancelButtonText: '保留数据' })
    removeData = true
  } catch { removeData = false }
  installingDb.value = 'uninstall'
  try {
    const r: any = await api.post('/dbmgr/uninstall', { engine, remove_data: removeData }, { timeout: 620000 })
    ElMessage.success(r?.message || '卸载完成')
    await loadStatus()
    loadCfg()
    loadData()
  } finally {
    installingDb.value = false
  }
}

async function ask(title: string, msg: string, _default = ''): Promise<string | null> {
  try {
    const r: any = await ElMessageBox.prompt(msg, title)
    return r.value
  } catch {
    return null
  }
}

async function askPwd(title: string): Promise<string | null> {
  try {
    const r: any = await ElMessageBox.prompt('输入密码（至少 6 位）', title, { inputType: 'password' })
    return r.value
  } catch {
    return null
  }
}

async function createDb(e: string) {
  const name = await ask('创建数据库', '输入数据库名（字母数字下划线）')
  if (!name) return
  await api.post(`/dbmgr/${e}/databases`, { name })
  ElMessage.success('已创建')
  loadData()
}

async function dropDb(e: string, name: string) {
  try {
    await ElMessageBox.confirm(`确认删除数据库「${name}」？数据不可恢复！`, '高危操作', { type: 'error' })
  } catch {
    return
  }
  await api.delete(`/dbmgr/${e}/databases/${name}`)
  loadData()
}

async function createUser(e: string) {
  const name = await ask('创建用户', e === 'mysql' ? '输入用户名' : '输入角色名')
  if (!name) return
  const pwd = await askPwd('创建用户')
  if (!pwd) return
  let host = '%'
  if (e === 'mysql') {
    const h = await ask('访问主机', '允许连接的主机（% 为任意）', '%')
    if (h == null) return
    host = h
  }
  if (e === 'mysql') await api.post('/dbmgr/mysql/users', { name, host, password: pwd })
  else await api.post('/dbmgr/pg/users', { name, password: pwd })
  ElMessage.success('已创建')
  loadData()
}

async function dropUser(e: string, row: any) {
  try {
    await ElMessageBox.confirm(`确认删除用户「${e === 'mysql' ? row.user + '@' + row.host : row}」？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  if (e === 'mysql') await api.delete(`/dbmgr/mysql/users/${row.user}/${row.host}`)
  else await api.delete(`/dbmgr/pg/users/${row}`)
  loadData()
}

async function grant(e: string) {
  const user = await ask('授权', '输入用户名')
  if (!user) return
  const db = await ask('授权', '输入数据库名')
  if (!db) return
  let host = '%'
  if (e === 'mysql') {
    const h = await ask('用户主机', '用户的主机（如 % 或 localhost）', '%')
    if (h == null) return
    host = h
  }
  if (e === 'mysql') await api.post('/dbmgr/mysql/grant', { user, host, db })
  else await api.post('/dbmgr/pg/grant', { user, db })
  ElMessage.success('已授权')
}

watch(engine, () => loadData())
onMounted(loadBackups)
onMounted(async () => {
  await loadStatus()
  await loadCfg()
  loadData()
})
</script>

<style scoped>
.wm-row-stats { font-size: 12px; }
.wm-dd-icon { margin-left: 4px; font-size: 10px; }
</style>
