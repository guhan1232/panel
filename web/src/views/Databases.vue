<template>
  <el-card>
    <el-tabs v-model="engine">
      <el-tab-pane label="MySQL" name="mysql">
        <el-form inline class="cfg">
          <el-form-item label="主机"><el-input v-model="mysql.host" style="width: 130px" /></el-form-item>
          <el-form-item label="端口"><el-input-number v-model="mysql.port" :min="1" controls-position="right" style="width: 110px" /></el-form-item>
          <el-form-item label="用户"><el-input v-model="mysql.user" style="width: 120px" /></el-form-item>
          <el-form-item label="密码"><el-input v-model="mysql.password" type="password" show-password style="width: 140px" /></el-form-item>
          <el-button @click="saveCfg('mysql')">保存配置</el-button>
          <el-button type="primary" @click="testConn('mysql')">测试连接</el-button>
        </el-form>
        <div class="toolbar">
          <el-button size="small" type="primary" @click="createDb('mysql')">创建数据库</el-button>
          <el-button size="small" @click="createUser('mysql')">创建用户</el-button>
          <el-button size="small" @click="grant('mysql')">授权用户到库</el-button>
          <el-button size="small" @click="loadData">刷新</el-button>
        </div>
        <el-row :gutter="16">
          <el-col :span="12">
            <h4>数据库</h4>
            <el-table :data="mysqlData.dbs" size="small" max-height="360">
              <el-table-column label="库名" prop="name" min-width="160" />
              <el-table-column label="操作" width="90">
                <template #default="{ row }"><el-button size="small" type="danger" @click="dropDb('mysql', row.name)">删除</el-button></template>
              </el-table-column>
            </el-table>
          </el-col>
          <el-col :span="12">
            <h4>用户</h4>
            <el-table :data="mysqlData.users" size="small" max-height="360">
              <el-table-column label="用户" prop="user" min-width="110" />
              <el-table-column label="主机" prop="host" min-width="110" />
              <el-table-column label="操作" width="90">
                <template #default="{ row }"><el-button size="small" type="danger" @click="dropUser('mysql', row)">删除</el-button></template>
              </el-table-column>
            </el-table>
          </el-col>
        </el-row>
      </el-tab-pane>

      <el-tab-pane label="PostgreSQL" name="pg">
        <el-form inline class="cfg">
          <el-form-item label="主机"><el-input v-model="pg.host" style="width: 130px" /></el-form-item>
          <el-form-item label="端口"><el-input-number v-model="pg.port" :min="1" controls-position="right" style="width: 110px" /></el-form-item>
          <el-form-item label="用户"><el-input v-model="pg.user" style="width: 120px" /></el-form-item>
          <el-form-item label="密码"><el-input v-model="pg.password" type="password" show-password style="width: 140px" /></el-form-item>
          <el-button @click="saveCfg('pg')">保存配置</el-button>
          <el-button type="primary" @click="testConn('pg')">测试连接</el-button>
        </el-form>
        <div class="toolbar">
          <el-button size="small" type="primary" @click="createDb('pg')">创建数据库</el-button>
          <el-button size="small" @click="createUser('pg')">创建用户</el-button>
          <el-button size="small" @click="grant('pg')">授权用户到库</el-button>
          <el-button size="small" @click="loadData">刷新</el-button>
        </div>
        <el-row :gutter="16">
          <el-col :span="12">
            <h4>数据库</h4>
            <el-table :data="pgData.dbs" size="small" max-height="360">
              <el-table-column label="库名" min-width="160"><template #default="{ row }">{{ row }}</template></el-table-column>
              <el-table-column label="操作" width="90">
                <template #default="{ row }"><el-button size="small" type="danger" @click="dropDb('pg', row)">删除</el-button></template>
              </el-table-column>
            </el-table>
          </el-col>
          <el-col :span="12">
            <h4>用户</h4>
            <el-table :data="pgData.users" size="small" max-height="360">
              <el-table-column label="角色" min-width="160"><template #default="{ row }">{{ row }}</template></el-table-column>
              <el-table-column label="操作" width="90">
                <template #default="{ row }"><el-button size="small" type="danger" @click="dropUser('pg', row)">删除</el-button></template>
              </el-table-column>
            </el-table>
          </el-col>
        </el-row>
      </el-tab-pane>
    </el-tabs>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const engine = ref('mysql')
const mysql = reactive<any>({ host: '127.0.0.1', port: 3306, user: 'root', password: '' })
const pg = reactive<any>({ host: '127.0.0.1', port: 5432, user: 'postgres', password: '', ssl_mode: 'disable' })
const mysqlData = reactive<any>({ dbs: [], users: [] })
const pgData = reactive<any>({ dbs: [], users: [] })

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
  try {
    if (engine.value === 'mysql') {
      mysqlData.dbs = (await api.get('/dbmgr/mysql/databases')) as any
      mysqlData.users = (await api.get('/dbmgr/mysql/users')) as any
    } else {
      pgData.dbs = (await api.get('/dbmgr/pg/databases')) as any
      pgData.users = (await api.get('/dbmgr/pg/users')) as any
    }
  } catch {
    /* 连接未配置时忽略 */
  }
}

async function ask(title: string, msg: string): Promise<string | null> {
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
onMounted(async () => {
  await loadCfg()
  loadData()
})
</script>
