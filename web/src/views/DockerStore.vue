<template>
  <div class="wm-page store-page">
    <a-tabs v-model:activeKey="activeTab" class="store-tabs">
      <!-- 应用商店 -->
      <a-tab-pane key="apps" tab="应用商店">
        <WmSectionCard dense class="store-control">
          <div class="store-control__row">
            <div class="store-control__stats">
              <div class="store-stat"><span>Docker</span><b>{{ docker.installed ? (docker.compose ? '就绪' : '缺 compose') : '未安装' }}</b></div>
              <div class="store-stat"><span>容器模板</span><b>{{ templates.length }}</b></div>
              <div class="store-stat"><span>已部署</span><b>{{ apps.length }}</b></div>
            </div>
            <div class="store-control__actions">
              <a-input v-model:value="keyword" placeholder="搜索容器模板" allow-clear style="width: 200px">
                <template #prefix><SearchOutlined /></template>
              </a-input>
              <a-radio-group v-model:value="sourceFilter" size="small">
                <a-radio-button value="all">全部</a-radio-button>
                <a-radio-button value="builtin">内置</a-radio-button>
                <a-radio-button value="remote">镜像源</a-radio-button>
              </a-radio-group>
              <a-button size="small" @click="load"><ReloadOutlined />刷新</a-button>
            </div>
          </div>
        </WmSectionCard>
        <a-row :gutter="16" class="store-grid">
          <a-col :xs="24" :xl="16">
            <WmSectionCard title="容器模板" description="Docker Compose 编排，部署时可为镜像选择加速源">
              <template #actions>
                <a-button v-if="!docker.installed" size="small" data-tone="warning" :loading="installing" @click="installDocker">一键安装 Docker</a-button>
                <a-button size="small" :disabled="!docker.installed || !docker.compose" @click="openCustom">部署自定义 Compose</a-button>
                <span class="wm-text-muted">显示 {{ filteredTemplates.length }} / {{ templates.length }}</span>
              </template>
              <a-row :gutter="[12, 12]" v-if="filteredTemplates.length">
                <a-col v-for="t in filteredTemplates" :key="t.key" :xs="24" :sm="12" :lg="8">
                  <div class="store-card">
                    <div class="store-card__head">
                      <span class="store-card__avatar store-card__avatar--docker">{{ (t.name || '?').slice(0, 1) }}</span>
                      <div class="store-card__title">
                        <b class="wm-truncate">{{ t.name }}</b>
                        <span v-if="t.version" class="store-card__ver">v{{ t.version }}</span>
                      </div>
                    </div>
                    <p class="store-card__desc">{{ t.desc }}</p>
                    <div class="store-card__tags">
                      <WmStatusPill :label="t.source === 'remote' ? '镜像源' : '内置'" :tone="t.source === 'remote' ? 'danger' : 'success'" :dot="false" outline />
                    </div>
                    <div class="store-card__foot">
                      <a-button type="primary" size="small" :disabled="!docker.installed || !docker.compose" @click="openInstall(t)">部署</a-button>
                    </div>
                  </div>
                </a-col>
              </a-row>
              <WmEmptyState v-else compact title="没有匹配的模板" description="试试更换关键词，或把来源筛选切回「全部」。" />
            </WmSectionCard>
          </a-col>
          <a-col :xs="24" :xl="8">
            <WmSectionCard title="已部署" :description="`${apps.length} 个实例`">
              <a-list v-if="apps.length" :data-source="apps" size="small" class="store-list">
                <template #renderItem="{ item }">
                  <a-list-item class="store-list__item">
                    <div class="store-list__meta">
                      <b class="wm-truncate">{{ item.app.name }}</b>
                      <span class="wm-text-muted wm-truncate">{{ item.app.app_key }}</span>
                    </div>
                    <template #actions>
                      <a-button type="link" size="small" @click="viewLogs(item)">日志</a-button>
                    </template>
                  </a-list-item>
                </template>
              </a-list>
              <WmEmptyState v-else compact title="还没有部署容器" description="从左侧卡片选择模板并点击部署。" />
            </WmSectionCard>
            <WmSectionCard title="运行环境" description="Docker 与镜像加速状态">
              <div class="store-env">
                <div><span>引擎</span><WmStatusPill :label="docker.installed ? '已安装' : '未安装'" :tone="docker.installed ? 'success' : 'danger'" /></div>
                <div><span>compose 插件</span><WmStatusPill :label="docker.compose ? '就绪' : '缺失'" :tone="docker.compose ? 'success' : 'warning'" /></div>
                <div><span>模板来源</span><b>内置 {{ builtinCount }} · 镜像源 {{ remoteCount }}</b></div>
              </div>
              <WmNotice tone="neutral" class="wm-mt-4">镜像加速写入 daemon.json，修改前自动备份；保存后需重启 Docker，运行中的容器会短暂中断。</WmNotice>
            </WmSectionCard>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- 总览 -->
      <a-tab-pane key="overview" tab="总览">
        <WmSectionCard title="Docker 总览" description="引擎版本、资源与容器统计">
          <template #actions><a-button size="small" @click="loadOverview"><ReloadOutlined />刷新</a-button></template>
          <WmEmptyState v-if="overview && overview.installed === false" compact title="Docker 未安装" description="到「应用商店」标签一键安装 Docker 后使用全部功能。" />
          <template v-else>
            <div class="ov-stats">
              <div class="ov-stat"><span>运行中容器</span><b>{{ overview.running ?? '-' }}</b></div>
              <div class="ov-stat"><span>已停止容器</span><b>{{ overview.stopped ?? '-' }}</b></div>
              <div class="ov-stat"><span>本地镜像</span><b>{{ overview.images ?? '-' }}</b></div>
              <div class="ov-stat"><span>服务器版本</span><b>{{ srvVersion }}</b></div>
              <div class="ov-stat"><span>存储驱动</span><b>{{ overview.info?.Driver || '-' }}</b></div>
              <div class="ov-stat"><span>容器根目录</span><b class="wm-truncate">{{ overview.info?.DockerRootDir || '-' }}</b></div>
            </div>
            <WmTable :data="statsRows" v-if="statsRows.length" size="small" class="wm-mt-4" empty-title="">
              <WmColumn label="容器" min-width="180"><template #default="{ row }"><b>{{ row.Name }}</b></template></WmColumn>
              <WmColumn label="CPU" width="100"><template #default="{ row }">{{ row.CPUPerc }}</template></WmColumn>
              <WmColumn label="内存" width="150"><template #default="{ row }">{{ row.MemUsage }} / {{ row.MemPerc }}</template></WmColumn>
              <WmColumn label="网络 I/O" min-width="160"><template #default="{ row }"><span class="wm-mono">{{ row.NetIO }}</span></template></WmColumn>
              <WmColumn label="磁盘 I/O" min-width="160"><template #default="{ row }"><span class="wm-mono">{{ row.BlockIO }}</span></template></WmColumn>
            </WmTable>
          </template>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 网站 -->
      <a-tab-pane key="sites" tab="网站">
        <WmSectionCard title="容器站点" description="发布了端口映射的容器（建站类应用一目了然）" flush>
          <template #actions><a-button size="small" @click="loadSites"><ReloadOutlined />刷新</a-button></template>
          <WmEmptyState v-if="sites && sites.installed === false" compact title="Docker 未安装" description="先安装 Docker。" />
          <WmTable v-else :data="sitesRows" v-loading="sitesLoading" empty-title="没有发布端口的容器" empty-description="部署带端口映射的容器后自动出现在这里。">
            <WmColumn label="容器" min-width="160"><template #default="{ row }"><b>{{ row.name }}</b></template></WmColumn>
            <WmColumn label="镜像" min-width="200"><template #default="{ row }"><span class="wm-mono">{{ row.image }}</span></template></WmColumn>
            <WmColumn label="状态" width="120"><template #default="{ row }"><WmStatusPill :label="stateLabel(row.state)" :tone="row.state === 'running' ? 'success' : 'danger'" :dot="row.state === 'running'" /></template></WmColumn>
            <WmColumn label="端口映射" min-width="260"><template #default="{ row }"><span class="wm-mono">{{ row.ports || '-' }}</span></template></WmColumn>
          </WmTable>
          <WmNotice tone="neutral" class="wm-mt-4">如需域名与 HTTPS，可在「网站」模块为容器端口创建反向代理站点。</WmNotice>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 容器 -->
      <a-tab-pane key="containers" tab="容器">
        <WmSectionCard title="容器列表" description="docker ps -a 全量容器，支持启停/日志/删除" flush>
          <template #actions>
            <a-button size="small" @click="loadContainers"><ReloadOutlined />刷新</a-button>
          </template>
          <WmEmptyState v-if="containers && containers.installed === false" compact title="Docker 未安装" description="先安装 Docker。" />
          <WmTable v-else :data="ctRows" v-loading="ctLoading" empty-title="没有容器" empty-description="先部署 Compose 应用或 docker run。">
            <WmColumn label="名称" min-width="160"><template #default="{ row }"><b>{{ row.Names }}</b></template></WmColumn>
            <WmColumn label="镜像" min-width="200"><template #default="{ row }"><span class="wm-mono">{{ row.Image }}</span></template></WmColumn>
            <WmColumn label="状态" width="130"><template #default="{ row }"><WmStatusPill :label="stateLabel(row.State)" :tone="row.State === 'running' ? 'success' : row.State === 'exited' ? 'danger' : 'warning'" :dot="row.State === 'running'" /></template></WmColumn>
            <WmColumn label="端口" min-width="180"><template #default="{ row }"><span class="wm-mono wm-truncate">{{ row.Ports || '-' }}</span></template></WmColumn>
            <WmColumn label="操作" width="280" fixed="right">
              <template #default="{ row }">
                <a-button v-if="row.State !== 'running'" size="small" data-tone="success" @click="ctAct(row, 'start')">启动</a-button>
                <a-button v-else size="small" data-tone="warning" @click="ctAct(row, 'stop')">停止</a-button>
                <a-button size="small" @click="ctAct(row, 'restart')">重启</a-button>
                <a-button size="small" @click="ctLogs(row)">日志</a-button>
                <a-button size="small" data-tone="danger" @click="ctRemove(row)">删除</a-button>
              </template>
            </WmColumn>
          </WmTable>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 线上镜像 -->
      <a-tab-pane key="remote-images" tab="线上镜像">
        <WmSectionCard title="镜像搜索与拉取" description="Docker Hub 搜索，一键拉取到本地">
          <WmToolbar>
            <a-input v-model:value="searchTerm" placeholder="如 nginx / redis / postgres" style="width: 260px" @keyup.enter="doSearch" />
            <a-button type="primary" @click="doSearch">搜索</a-button>
          </WmToolbar>
          <WmTable :data="searchRows" v-loading="searchLoading" size="small" empty-title="输入关键词搜索线上镜像" empty-description="">
            <WmColumn label="镜像" min-width="200"><template #default="{ row }"><b class="wm-mono">{{ row.name }}</b><p class="wm-text-muted">{{ row.description }}</p></template></WmColumn>
            <WmColumn label="Stars" width="100"><template #default="{ row }">{{ row.star_count }}</template></WmColumn>
            <WmColumn label="官方" width="90"><template #default="{ row }"><WmStatusPill v-if="row.official" label="OFFICIAL" tone="success" :dot="false" /></template></WmColumn>
            <WmColumn label="操作" width="120"><template #default="{ row }"><a-button size="small" type="primary" :loading="pulling === row.name" @click="pullImage(row.name)">拉取</a-button></template></WmColumn>
          </WmTable>
          <a-modal v-model:open="pullDlg" :title="`拉取 ${pullImageName}`" width="720px" :footer="null">
            <WmCodeBlock :content="pullOutput" :label="`${pullImageName} / pull`" />
          </a-modal>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 本地镜像 -->
      <a-tab-pane key="images" tab="本地镜像">
        <WmSectionCard title="镜像列表" description="docker images 全量镜像与清理" flush>
          <template #actions>
            <a-button size="small" data-tone="danger" @click="pruneImages">清理未使用镜像</a-button>
            <a-button size="small" @click="loadImages"><ReloadOutlined />刷新</a-button>
          </template>
          <WmTable :data="imgRows" v-loading="imgLoading" empty-title="没有镜像" empty-description="部署应用或从「线上镜像」拉取。">
            <WmColumn label="仓库" min-width="240"><template #default="{ row }"><span class="wm-mono">{{ row.Repository }}</span></template></WmColumn>
            <WmColumn label="标签" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.Tag }}</span></template></WmColumn>
            <WmColumn label="大小" width="110"><template #default="{ row }">{{ row.Size }}</template></WmColumn>
            <WmColumn label="创建于" width="150"><template #default="{ row }">{{ row.CreatedSince }}</template></WmColumn>
          </WmTable>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 容器编排 -->
      <a-tab-pane key="compose" tab="容器编排">
        <WmSectionCard title="已部署编排" description="Compose 实例：编辑 YAML 即时生效" flush>
          <template #actions>
            <a-button size="small" :disabled="!docker.installed || !docker.compose" @click="openCustom">部署自定义 Compose</a-button>
            <a-button size="small" @click="load"><ReloadOutlined />刷新</a-button>
          </template>
          <WmTable :data="apps" v-loading="loading" empty-title="还没有编排应用" empty-description="在「应用商店」部署模板，或部署自定义 Compose。">
            <WmColumn label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></WmColumn>
            <WmColumn label="应用" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></WmColumn>
            <WmColumn label="状态" min-width="220"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></WmColumn>
            <WmColumn label="部署时间" width="160"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.app.created_at) }}</span></template></WmColumn>
            <WmColumn label="操作" width="330" fixed="right">
              <template #default="{ row }">
                <a-button size="small" data-tone="success" @click="act(row, 'start')">启动</a-button>
                <a-button size="small" data-tone="warning" @click="act(row, 'stop')">停止</a-button>
                <a-button size="small" @click="act(row, 'restart')">重启</a-button>
                <a-button size="small" @click="viewLogs(row)">日志</a-button>
                <a-button size="small" @click="editCompose(row)">编排</a-button>
                <a-button size="small" data-tone="danger" @click="uninstall(row)">卸载</a-button>
              </template>
            </WmColumn>
          </WmTable>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 网络 -->
      <a-tab-pane key="networks" tab="网络">
        <WmSectionCard title="Docker 网络" description="bridge/host/none 为内置网络不可删除" flush>
          <template #actions>
            <a-button size="small" type="primary" @click="netDlg = true">创建网络</a-button>
            <a-button size="small" @click="loadNetworks"><ReloadOutlined />刷新</a-button>
          </template>
          <WmTable :data="netRows" v-loading="netLoading" empty-title="没有网络" empty-description="">
            <WmColumn label="名称" min-width="180"><template #default="{ row }"><b class="wm-mono">{{ row.Name }}</b></template></WmColumn>
            <WmColumn label="驱动" width="120"><template #default="{ row }">{{ row.Driver }}</template></WmColumn>
            <WmColumn label="范围" width="110"><template #default="{ row }">{{ row.Scope }}</template></WmColumn>
            <WmColumn label="操作" width="100"><template #default="{ row }"><a-button v-if="!['bridge','host','none'].includes(row.Name)" size="small" data-tone="danger" @click="delNetwork(row.Name)">删除</a-button></template></WmColumn>
          </WmTable>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 存储卷 -->
      <a-tab-pane key="volumes" tab="存储卷">
        <WmSectionCard title="Docker 存储卷" description="数据卷列表与删除（删除前确认容器未使用）" flush>
          <template #actions><a-button size="small" @click="loadVolumes"><ReloadOutlined />刷新</a-button></template>
          <WmTable :data="volRows" v-loading="volLoading" empty-title="没有存储卷" empty-description="">
            <WmColumn label="名称" min-width="240"><template #default="{ row }"><b class="wm-mono">{{ row.Name }}</b></template></WmColumn>
            <WmColumn label="驱动" width="110"><template #default="{ row }">{{ row.Driver }}</template></WmColumn>
            <WmColumn label="操作" width="100"><template #default="{ row }"><a-button size="small" data-tone="danger" @click="delVolume(row.Name)">删除</a-button></template></WmColumn>
          </WmTable>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 仓库 -->
      <a-tab-pane key="registry" tab="仓库">
        <WmSectionCard title="镜像仓库登录" description="docker login 私有仓库；凭据由本机 credential store 保存，面板不落库">
          <a-form layout="vertical" style="max-width: 460px">
            <a-form-item label="仓库地址"><a-input v-model:value="regForm.registry" placeholder="如 registry.cn-hangzhou.aliyuncs.com" /></a-form-item>
            <a-form-item label="用户名"><a-input v-model:value="regForm.username" /></a-form-item>
            <a-form-item label="密码"><a-input-password v-model:value="regForm.password" /></a-form-item>
            <a-button type="primary" :loading="regLogining" @click="doLogin">登录仓库</a-button>
          </a-form>
          <WmNotice tone="neutral" class="wm-mt-4">登录成功后即可从该仓库拉取私有镜像（配合「线上镜像」页直接输入完整镜像地址拉取）。</WmNotice>
        </WmSectionCard>
      </a-tab-pane>

      <!-- 设置 -->
      <a-tab-pane key="settings" tab="设置">
        <WmSectionCard title="镜像源配置（daemon.json）" description="加速镜像源与私有仓库白名单">
          <a-form :label-col="{ style: { width: '130px' } }">
            <a-form-item label="加速镜像源">
              <a-textarea v-model:value="mirrorForm.registry_mirrors" :rows="5" placeholder="每行一个，如 https://docker.m.daocloud.io" />
              <div class="presets">
                <span class="hint">常用源（时效性强，按可用性自行取舍）：</span>
                <a-tag v-for="m in mirrorPresets" :key="m" size="small" class="preset-tag" @click="addPreset(m)">{{ m }}</a-tag>
              </div>
            </a-form-item>
            <a-form-item label="私有仓库白名单">
              <a-textarea v-model:value="mirrorForm.insecure_registries" :rows="3" placeholder="每行一个 HTTP 私有仓库，如 192.168.1.10:5000" />
            </a-form-item>
            <a-form-item v-if="mirrorInfo.live_mirrors && mirrorInfo.live_mirrors.length" label="当前生效">
              <div class="live-mirrors">
                <WmStatusPill v-for="m in mirrorInfo.live_mirrors" :key="m" :label="m" tone="success" :dot="false" mono />
              </div>
            </a-form-item>
            <a-form-item label="配置文件"><span class="hint wm-mono">{{ mirrorInfo.path }}</span><span class="hint">其余配置项保持不变，修改前自动备份</span></a-form-item>
            <a-button type="primary" :loading="mirrorSaving" @click="saveMirrors">保存并重启 Docker</a-button>
          </a-form>
        </WmSectionCard>
      </a-tab-pane>
    </a-tabs>

    <a-modal v-model:open="dlg" :title="`部署 ${cur?.name || ''}`" width="560px">
      <a-form :label-col="{ style: { width: '130px' } }">
        <a-form-item label="实例名称"><a-input v-model:value="instName" placeholder="如 my-redis" /></a-form-item>
        <a-form-item label="镜像源">
          <a-select v-model:value="instMirror" style="width: 100%">
            <a-select-option v-for="m in dockerMirrors" :key="m.id" :value="m.id">{{ m.name }}</a-select-option>
          </a-select>
          <div class="hint">部署时将 YAML 中的镜像改写为所选代理仓库拉取；也可在「设置」里设置全局 daemon.json 加速</div>
        </a-form-item>
        <a-form-item v-for="p in cur?.params || []" :key="p.key" :label="p.label">
          <a-input v-model:value="instParams[p.key]" :placeholder="p.def" :type="p.key.includes('pass') || p.key === 'password' ? 'password' : 'text'" />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="deploying" @click="deploy">部署（拉取镜像可能较慢）</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="logDlg" :title="`日志 - ${logName}`" width="820px">
      <WmCodeBlock :content="logContent" :label="`${logName} / logs`" />
    </a-modal>

    <a-modal v-model:open="composeDlg" :title="`编排编辑 - ${composeName}`" width="860px">
      <WmNotice tone="warning" class="wm-mb-4">保存后立即执行 docker compose up -d 应用变更，可能重建容器。</WmNotice>
      <a-textarea v-model:value="composeContent" :rows="20" class="wm-mono" style="font-family: Consolas, Monaco, monospace; font-size: 12px" />
      <template #footer>
        <a-button @click="composeDlg = false">取消</a-button>
        <a-button type="primary" :loading="composeSaving" @click="saveCompose">保存并应用</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="customDlg" title="部署自定义 Compose 应用" width="860px">
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="实例名称"><a-input v-model:value="customName" placeholder="如 my-stack" /></a-form-item>
        <a-form-item label="Compose YAML">
          <a-textarea v-model:value="customContent" :rows="16" style="font-family: Consolas, Monaco, monospace; font-size: 12px" placeholder='services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"' />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="customDlg = false">取消</a-button>
        <a-button type="primary" :loading="customDeploying" @click="deployCustom">部署</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="netDlg" title="创建 Docker 网络" @ok="createNetwork" ok-text="创建" cancel-text="取消">
      <a-form layout="vertical">
        <a-form-item label="网络名称"><a-input v-model:value="netForm.name" placeholder="如 mynet" /></a-form-item>
        <a-form-item label="驱动">
          <a-select v-model:value="netForm.driver">
            <a-select-option value="bridge">bridge（默认）</a-select-option>
            <a-select-option value="macvlan">macvlan</a-select-option>
            <a-select-option value="overlay">overlay（集群）</a-select-option>
          </a-select>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import api from '../api'
import { fmtTime } from '../utils'

const docker = ref<any>({})
const templates = ref<any[]>([])
const apps = ref<any[]>([])
const loading = ref(false)
const installing = ref(false)
const dlg = ref(false)
const deploying = ref(false)
const cur = ref<any>(null)
const instName = ref('')
const instMirror = ref('auto')
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

const activeTab = ref('apps')
const containers = ref<any>({})
const images = ref<any>({})
const ctLoading = ref(false)
const imgLoading = ref(false)
const composeDlg = ref(false)
const composeName = ref('')
const composeContent = ref('')
const composeSaving = ref(false)
const customDlg = ref(false)
const customName = ref('')
const customContent = ref('')
const customDeploying = ref(false)
const overview = ref<any>(null)
const sites = ref<any>(null)
const sitesRows = ref<any[]>([])
const sitesLoading = ref(false)
const netRows = ref<any[]>([])
const netLoading = ref(false)
const netDlg = ref(false)
const netForm = reactive({ name: '', driver: 'bridge' })
const volRows = ref<any[]>([])
const volLoading = ref(false)
const searchTerm = ref('')
const searchRows = ref<any[]>([])
const searchLoading = ref(false)
const pulling = ref('')
const pullDlg = ref(false)
const pullImageName = ref('')
const pullOutput = ref('')
const regForm = reactive({ registry: '', username: '', password: '' })
const regLogining = ref(false)

const ctRows = computed<any[]>(() => (Array.isArray(containers.value) ? containers.value : []))
const imgRows = computed<any[]>(() => (Array.isArray(images.value) ? images.value : []))
const srvVersion = computed(() => {
  try {
    return overview.value?.version?.Server?.Version || overview.value?.info?.ServerVersion || '-'
  } catch {
    return '-'
  }
})

function stateLabel(st: string) {
  const m: Record<string, string> = { running: '运行中', exited: '已退出', paused: '已暂停', created: '已创建', restarting: '重启中', dead: '异常' }
  return m[st] || st
}

async function loadContainers() {
  ctLoading.value = true
  try {
    containers.value = (await api.get('/apps/docker/containers')) as any
  } catch {
    containers.value = []
  } finally {
    ctLoading.value = false
  }
}

async function loadImages() {
  imgLoading.value = true
  try {
    images.value = (await api.get('/apps/docker/images')) as any
  } catch {
    images.value = []
  } finally {
    imgLoading.value = false
  }
}

async function loadOverview() {
  overview.value = null
  try {
    overview.value = await api.get('/apps/docker/overview')
  } catch {
    overview.value = { installed: false }
  }
  try {
    statsRows.value = (await api.get('/apps/docker/stats')) as any
  } catch {
    statsRows.value = []
  }
}
const statsRows = ref<any[]>([])

async function loadSites() {
  sitesLoading.value = true
  try {
    const d: any = await api.get('/apps/docker/sites')
    sites.value = Array.isArray(d) ? { ok: true } : d
    sitesRows.value = Array.isArray(d) ? d : []
  } finally {
    sitesLoading.value = false
  }
}

async function loadNetworks() {
  netLoading.value = true
  try {
    netRows.value = (await api.get('/apps/docker/networks')) as any
  } catch {
    netRows.value = []
  } finally {
    netLoading.value = false
  }
}

async function createNetwork() {
  if (!netForm.name) {
    ElMessage.warning('请填写网络名称')
    return
  }
  await api.post('/apps/docker/networks', { name: netForm.name, driver: netForm.driver })
  ElMessage.success('网络已创建')
  netDlg.value = false
  netForm.name = ''
  loadNetworks()
}

async function delNetwork(name: string) {
  try {
    await ElMessageBox.confirm(`确认删除网络「${name}」？`, '删除网络', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/apps/docker/networks/${encodeURIComponent(name)}`)
  ElMessage.success('已删除')
  loadNetworks()
}

async function loadVolumes() {
  volLoading.value = true
  try {
    volRows.value = (await api.get('/apps/docker/volumes')) as any
  } catch {
    volRows.value = []
  } finally {
    volLoading.value = false
  }
}

async function delVolume(name: string) {
  try {
    await ElMessageBox.confirm(`确认删除存储卷「${name}」？数据不可恢复！`, '删除存储卷', { type: 'error' })
  } catch {
    return
  }
  await api.delete(`/apps/docker/volumes/${encodeURIComponent(name)}`)
  ElMessage.success('已删除')
  loadVolumes()
}

async function doSearch() {
  if (!searchTerm.value.trim()) return
  searchLoading.value = true
  try {
    searchRows.value = (await api.get('/apps/docker/search', { params: { term: searchTerm.value.trim() } })) as any
  } catch {
    searchRows.value = []
  } finally {
    searchLoading.value = false
  }
}

async function pullImage(name: string) {
  pullImageName.value = name
  pullOutput.value = '开始拉取…'
  pullDlg.value = true
  pulling.value = name
  try {
    const d: any = await api.post('/apps/docker/pull', { image: name }, { timeout: 920000 })
    pullOutput.value = d.output || '拉取完成'
    ElMessage.success('拉取完成')
  } catch (e: any) {
    pullOutput.value = e?.message || '拉取失败'
  } finally {
    pulling.value = ''
    loadImages()
  }
}

async function doLogin() {
  if (!regForm.registry || !regForm.username || !regForm.password) {
    ElMessage.warning('请填写完整')
    return
  }
  regLogining.value = true
  try {
    await api.post('/apps/docker/login', { ...regForm }, { timeout: 90000 })
    ElMessage.success('仓库登录成功')
    regForm.password = ''
  } finally {
    regLogining.value = false
  }
}

async function switchTab(k: string | number) {
  if (k === 'containers') await loadContainers()
  else if (k === 'images') await loadImages()
  else if (k === 'overview') await loadOverview()
  else if (k === 'sites') await loadSites()
  else if (k === 'networks') await loadNetworks()
  else if (k === 'volumes') await loadVolumes()
}

async function ctAct(row: any, action: string) {
  await api.post(`/apps/docker/containers/${encodeURIComponent(row.Names)}/${action}`)
  ElMessage.success('已' + (action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'))
  loadContainers()
}

async function ctRemove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除容器「${row.Names}」？（数据卷不受影响）`, '删除容器', { type: 'warning' })
  } catch {
    return
  }
  await api.post(`/apps/docker/containers/${encodeURIComponent(row.Names)}/remove?force=1`)
  ElMessage.success('已删除')
  loadContainers()
}

async function ctLogs(row: any) {
  const d: any = await api.get(`/apps/docker/containers/${encodeURIComponent(row.Names)}/logs`, { params: { tail: 300 } })
  logName.value = row.Names
  logContent.value = d.logs
  logDlg.value = true
}

async function pruneImages() {
  try {
    await ElMessageBox.confirm('清理所有未被容器引用的镜像？不可恢复。', '清理镜像', { type: 'warning' })
  } catch {
    return
  }
  await api.post('/apps/docker/images/prune?force=1')
  ElMessage.success('已清理')
  loadImages()
}

async function editCompose(row: any) {
  const d: any = await api.get('/apps/docker/compose', { params: { name: row.app.name } })
  composeName.value = d.name
  composeContent.value = d.content
  composeDlg.value = true
}

async function saveCompose() {
  composeSaving.value = true
  try {
    await api.post('/apps/docker/compose', { name: composeName.value, content: composeContent.value }, { timeout: 900000 })
    ElMessage.success('已保存并应用')
    composeDlg.value = false
  } finally {
    composeSaving.value = false
  }
}

function openCustom() {
  customName.value = ''
  customContent.value = 'services:\n  web:\n    image: nginx:alpine\n    ports:\n      - "8080:80"'
  customDlg.value = true
}

async function deployCustom() {
  if (!customName.value) {
    ElMessage.warning('请填写实例名称')
    return
  }
  customDeploying.value = true
  try {
    await api.post('/apps/docker/compose-deploy', { name: customName.value, content: customContent.value }, { timeout: 900000 })
    ElMessage.success('部署完成')
    customDlg.value = false
    load()
    activeTab.value = 'compose'
  } finally {
    customDeploying.value = false
  }
}

const builtinCount = computed(() => templates.value.filter((t) => t.source !== 'remote').length)
const remoteCount = computed(() => templates.value.filter((t) => t.source === 'remote').length)

const keyword = ref('')
const sourceFilter = ref<'all' | 'builtin' | 'remote'>('all')
const filteredTemplates = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return templates.value.filter((t) => {
    if (sourceFilter.value === 'builtin' && t.source === 'remote') return false
    if (sourceFilter.value === 'remote' && t.source !== 'remote') return false
    if (!q) return true
    return [t.name, t.key, t.desc].some((v: any) => String(v || '').toLowerCase().includes(q))
  })
})

const mirrorPresets = [
  'https://docker.m.daocloud.io',
  'https://docker.1ms.run',
  'https://hub.rat.dev',
  'https://docker.nju.edu.cn',
  'https://docker.mirrors.ustc.edu.cn',
  'https://hub-mirror.c.163.com'
]
const mirrorSaving = ref(false)
const mirrorInfo = ref<any>({})
const mirrorForm = reactive({ registry_mirrors: '', insecure_registries: '' })

const dockerMirrors = computed(() => [
  { id: 'auto', name: '自动（优先加速源）' },
  { id: 'off', name: '官方直连' },
  ...mirrorsList.value.filter((m: any) => m.docker)
])
const mirrorsList = ref<any[]>([])

async function load() {
  loading.value = true
  try {
    docker.value = await api.get('/apps/docker/status')
    templates.value = ((await api.get('/apps/templates')) as any).filter((t: any) => t.kind === 'compose')
    apps.value = ((await api.get('/apps')) as any).filter((a: any) => a.app.kind !== 'native' && a.app.kind !== 'script')
    try {
      mirrorsList.value = (await api.get('/apps/mirrors')) as any
    } catch {
      mirrorsList.value = []
    }
  } finally {
    loading.value = false
  }
}

async function openMirrors() {
  mirrorInfo.value = await api.get('/apps/docker/mirrors')
  mirrorForm.registry_mirrors = (mirrorInfo.value.registry_mirrors || []).join('\n')
  mirrorForm.insecure_registries = (mirrorInfo.value.insecure_registries || []).join('\n')
}

function addPreset(m: string) {
  const lines = mirrorForm.registry_mirrors.split('\n').map((s: string) => s.trim()).filter(Boolean)
  if (!lines.includes(m)) lines.push(m)
  mirrorForm.registry_mirrors = lines.join('\n')
}

async function saveMirrors() {
  try {
    await ElMessageBox.confirm(
      '保存需要重启 Docker 服务，运行中的容器会短暂中断（restart=always 的容器会自动恢复），继续？',
      '重启确认', { type: 'warning' }
    )
  } catch {
    return
  }
  mirrorSaving.value = true
  try {
    const d: any = await api.post('/apps/docker/mirrors', {
      registry_mirrors: mirrorForm.registry_mirrors.split('\n').map((s: string) => s.trim()).filter(Boolean),
      insecure_registries: mirrorForm.insecure_registries.split('\n').map((s: string) => s.trim()).filter(Boolean),
      restart: true
    }, { timeout: 180000 })
    ElMessage.success(d.restarted ? '已保存并重启 Docker' : '已保存（未重启，下次启动 Docker 生效）')
    mirrorInfo.value.live_mirrors = d.live_mirrors
  } finally {
    mirrorSaving.value = false
  }
}

function openInstall(t: any) {
  cur.value = t
  instName.value = t.key
  instMirror.value = 'auto'
  Object.keys(instParams).forEach((k) => delete instParams[k])
  for (const p of t.params || []) instParams[p.key] = p.def || ''
  dlg.value = true
}

async function deploy() {
  if (!instName.value) {
    ElMessage.warning('请填写实例名称')
    return
  }
  deploying.value = true
  try {
    await api.post('/apps', { app_key: cur.value.key, name: instName.value, mirror: instMirror.value, params: { ...instParams } }, { timeout: 900000 })
    ElMessage.success('部署完成')
    dlg.value = false
    load()
  } finally {
    deploying.value = false
  }
}

async function act(row: any, action: string) {
  await api.post(`/apps/${row.app.id}/${action}`, {}, { timeout: 900000 })
  ElMessage.success('已执行 ' + action)
  load()
}

async function viewLogs(row: any) {
  const d: any = await api.get(`/apps/${row.app.id}/logs`)
  logName.value = row.app.name
  logContent.value = d.content
  logDlg.value = true
}

async function uninstall(row: any) {
  let removeData = false
  try {
    await ElMessageBox.confirm(`确认卸载应用「${row.app.name}」？`, '卸载', { type: 'warning' })
  } catch {
    return
  }
  try {
    await ElMessageBox.confirm('是否同时删除数据目录（数据库文件等，不可恢复）？', '删除数据', {
      type: 'error', confirmButtonText: '删除数据', cancelButtonText: '保留数据'
    })
    removeData = true
  } catch {
    removeData = false
  }
  await api.post(`/apps/${row.app.id}/uninstall`, { remove_data: removeData })
  ElMessage.success('已卸载')
  load()
}

async function installDocker() {
  try {
    await ElMessageBox.confirm('将通过官方脚本安装 Docker（需要外网，耗时数分钟），继续？', '安装 Docker', { type: 'warning' })
  } catch {
    return
  }
  installing.value = true
  try {
    await api.post('/apps/docker/install', {}, { timeout: 1800000 })
    ElMessage.success('Docker 安装完成')
    load()
  } finally {
    installing.value = false
  }
}

onMounted(() => {
  load()
  openMirrors()
})
watch(activeTab, switchTab)
</script>

<style scoped>
.store-tabs :deep(.ant-tabs-content-holder) {
  padding-top: 8px;
}
.store-control {
  margin-bottom: 12px;
}
.store-control__row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.store-control__stats {
  display: flex;
  gap: 24px;
}
.store-stat span {
  display: block;
  font-size: 12px;
  color: var(--wm-text-2, #8492a6);
}
.store-stat b {
  font-size: 18px;
}
.ov-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}
.ov-stat {
  padding: 12px 14px;
  border: 1px solid var(--wm-border, #e5e8ef);
  border-radius: 8px;
  background: var(--wm-card, #fff);
}
.ov-stat span {
  display: block;
  font-size: 12px;
  color: var(--wm-text-2, #8492a6);
}
.ov-stat b {
  font-size: 18px;
}
.store-card {
  border: 1px solid var(--wm-border, #e5e8ef);
  border-radius: 10px;
  padding: 14px;
  background: var(--wm-card, #fff);
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
}
.store-card__head {
  display: flex;
  gap: 10px;
  align-items: center;
}
.store-card__avatar {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  color: #fff;
  background: #3a7afe;
}
.store-card__avatar--docker {
  background: #0b57d0;
}
.store-card__title {
  display: flex;
  gap: 6px;
  align-items: baseline;
  min-width: 0;
}
.store-card__ver {
  font-size: 12px;
  color: var(--wm-text-2, #8492a6);
}
.store-card__desc {
  font-size: 12px;
  color: var(--wm-text-2, #8492a6);
  margin: 0;
  flex: 1;
}
.store-card__tags {
  display: flex;
  gap: 6px;
}
.store-card__foot {
  display: flex;
  justify-content: flex-end;
}
.store-env > div {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 0;
}
.store-list__item {
  padding: 6px 0;
}
.store-list__meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.mono {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.hint {
  color: var(--wm-text-2, #8492a6);
  font-size: 12px;
  line-height: 1.7;
  margin-top: 4px;
}
.presets {
  margin-top: 8px;
  line-height: 2;
}
.preset-tag {
  cursor: pointer;
  margin-right: 6px;
}
.live-mirrors {
  line-height: 1.8;
}
</style>
