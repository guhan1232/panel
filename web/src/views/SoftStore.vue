<template>
  <div class="wm-page store-page ops-page">
    <!-- 顶部命令栏 -->
    <section class="ops-commandbar">
      <div class="ops-commandbar__group">
        <a-input v-model:value="keyword" placeholder="搜索软件、脚本或运行时" allow-clear style="width: 240px">
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <template v-if="storeTab === 'apps'">
          <a-segmented v-model:value="catFilter" size="small" :options="catOptions" />
          <a-segmented v-model:value="sourceFilter" size="small" :options="[
            { value: 'all', label: `全部 ${templates.length}` },
            { value: 'builtin', label: `内置 ${builtinCount}` },
            { value: 'remote', label: `仓库 ${remoteCount}` },
            { value: 'local', label: `本地 ${localCount}` }
          ]" />
        </template>
      </div>
      <div class="ops-commandbar__group ops-commandbar__group--end">
        <a-button v-if="storeTab === 'apps'" size="small" type="primary" ghost @click="openScriptDlg()">上传脚本</a-button>
        <a-button v-if="storeTab === 'sources'" size="small" type="primary" @click="openSrcDlg()">添加仓库源</a-button>
        <a-button size="small" @click="loadAll"><ReloadOutlined />刷新</a-button>
      </div>
    </section>

    <a-tabs v-model:activeKey="storeTab" tab-position="top" class="store-tabs ops-tabs">
      <!-- ============ Tab 1 应用商店 ============ -->
      <a-tab-pane key="apps">
        <template #tab>
          <span class="ops-tab"><AppstoreOutlined /><span class="ops-tab__label">应用商店</span><span class="ops-tab__count">{{ templates.length }}</span></span>
        </template>
        <section class="ops-panel">
          <header class="ops-panel__head">
            <div>
              <h3 class="ops-panel__title">软件目录</h3>
              <p class="ops-panel__desc">内置应用、仓库托管脚本与本地三方脚本包一键安装；容器类见 Docker 页，系统包可选镜像源。</p>
            </div>
            <div class="ops-panel__actions">
              <WmStatusPill :label="`显示 ${filteredTemplates.length} / ${templates.length}`" tone="neutral" :dot="false" outline />
            </div>
          </header>
          <div class="ops-panel__body">
            <div class="ops-card-grid" v-if="filteredTemplates.length">
              <article v-for="t in filteredTemplates" :key="t.key" class="store-card">
                <div class="store-card__head">
                  <span class="store-card__avatar" :class="{ 'store-card__avatar--local': t.source === 'local' }">{{ t.icon || (t.name || '?').slice(0, 1) }}</span>
                  <div class="store-card__title">
                    <b class="wm-truncate">{{ t.name }}</b>
                    <span v-if="t.version" class="store-card__ver">v{{ t.version }}</span>
                  </div>
                </div>
                <p class="store-card__desc">{{ t.desc || '（暂无描述）' }}</p>
                <div class="store-card__tags">
                  <WmStatusPill :label="kindLabel(t.kind)" :tone="t.kind === 'package' ? 'success' : t.kind === 'native' ? 'warning' : 'info'" :dot="false" outline />
                  <WmStatusPill :label="sourceLabel(t)" :tone="t.source === 'builtin' ? 'success' : t.source === 'local' ? 'warning' : 'processing'" :dot="false" outline />
                  <WmStatusPill v-if="t.category" :label="t.category" tone="neutral" :dot="false" outline />
                </div>
                <div class="store-card__foot">
                  <a-button type="primary" size="small" @click="openInstall(t)">安装</a-button>
                  <a-button v-if="t.source === 'local'" size="small" @click="editScript(String(t.key).replace('custom:', ''))">编辑</a-button>
                </div>
              </article>
            </div>
            <WmEmptyState v-else compact title="没有匹配的软件" description="试试更换关键词、切换分类，或到「脚本管理」上传自己的脚本包。" />
          </div>
        </section>
      </a-tab-pane>

      <!-- ============ Tab 2 脚本管理 ============ -->
      <a-tab-pane key="scripts">
        <template #tab>
          <span class="ops-tab"><FileTextOutlined /><span class="ops-tab__label">脚本管理</span><span class="ops-tab__count">{{ customScripts.length }}</span></span>
        </template>
        <section class="ops-panel">
          <header class="ops-panel__head">
            <div>
              <h3 class="ops-panel__title">三方脚本包</h3>
              <p class="ops-panel__desc">上传自带元数据与参数声明的 bash 脚本包，出现在「应用商店」可直接安装；直接运行会留痕可查日志。</p>
            </div>
            <div class="ops-panel__actions">
              <a-button type="primary" size="small" @click="openScriptDlg()">上传脚本包</a-button>
            </div>
          </header>
          <div class="ops-panel__body">
            <div class="ops-card-grid" v-if="customScripts.length">
              <article v-for="s in customScripts" :key="s.name" class="store-card">
                <div class="store-card__head">
                  <span class="store-card__avatar store-card__avatar--local">{{ s.icon || (s.title || s.name).slice(0, 1) }}</span>
                  <div class="store-card__title">
                    <b class="wm-truncate">{{ s.title || s.name }}</b>
                    <span v-if="s.version" class="store-card__ver">v{{ s.version }}</span>
                  </div>
                </div>
                <p class="store-card__desc">{{ s.desc || s.head || 'bash 脚本' }}</p>
                <div class="store-card__tags">
                  <WmStatusPill v-if="s.category" :label="s.category" tone="neutral" :dot="false" outline />
                  <WmStatusPill :label="`${(s.params || []).length} 个参数`" tone="info" :dot="false" outline />
                </div>
                <div class="store-card__foot store-card__foot--wrap">
                  <a-button type="primary" size="small" @click="openInstall({ key: 'custom:' + s.name, name: s.title || s.name, desc: s.desc, kind: 'script', params: s.params })">安装</a-button>
                  <a-button size="small" @click="editScript(s.name)">编辑</a-button>
                  <a-button type="link" size="small" @click="viewScriptLog(s.name)">日志</a-button>
                  <a-button type="link" size="small" danger @click="delScript(s.name)">删除</a-button>
                </div>
              </article>
            </div>
            <WmEmptyState v-else compact title="还没有脚本包" description="点击右上角「上传脚本包」：填写名称、描述、参数声明并粘贴 bash 内容。" />
            <WmNotice tone="neutral" class="wm-mt-4">
              脚本以 bash 在面板服务器执行（限时 30 分钟，环境变量注入 PANEL_APP_NAME）；参数以「双花括号包裹参数名」占位符替换进脚本，安装时按声明生成表单。
            </WmNotice>
          </div>
        </section>
      </a-tab-pane>

      <!-- ============ Tab 3 应用源 ============ -->
      <a-tab-pane key="sources">
        <template #tab>
          <span class="ops-tab"><CloudOutlined /><span class="ops-tab__label">应用源</span><span class="ops-tab__count">{{ appSources.length }}</span></span>
        </template>
        <section class="ops-panel">
          <header class="ops-panel__head">
            <div>
              <h3 class="ops-panel__title">仓库源</h3>
              <p class="ops-panel__desc">从 git 仓库清单（catalog.json）获取软件安装脚本与源码声明；支持添加任意 http(s) 清单地址，启用后合并进「应用商店」。</p>
            </div>
            <div class="ops-panel__actions">
              <a-button type="primary" size="small" @click="openSrcDlg()">添加仓库源</a-button>
            </div>
          </header>
          <div class="ops-panel__body">
            <div class="store-src-list" v-if="appSources.length">
              <article v-for="s in appSources" :key="s.id" class="store-src" :class="{ 'store-src--off': !s.enabled }">
                <div class="store-src__main">
                  <div class="store-src__title">
                    <b>{{ s.name }}</b>
                    <WmStatusPill v-if="s.builtin" label="内置" tone="success" :dot="false" outline />
                    <WmStatusPill :label="s.enabled ? '已启用' : '已停用'" :tone="s.enabled ? 'success' : 'neutral'" :dot="false" />
                    <WmStatusPill v-if="s.err" label="刷新失败" tone="danger" :dot="false" />
                  </div>
                  <p class="store-src__url wm-mono wm-truncate">{{ s.url }}</p>
                  <p class="store-src__meta">
                    {{ s.count }} 个应用 · 更新于 {{ s.updated || '从未' }}
                    <span v-if="s.err" class="store-src__err">（{{ s.err }}）</span>
                  </p>
                </div>
                <div class="store-src__actions">
                  <a-switch :checked="s.enabled" size="small" @change="(v: any) => toggleSource(s, v)" />
                  <a-button size="small" :loading="refreshingId === s.id" @click="refreshSource(s)">刷新</a-button>
                  <a-button size="small" @click="openSrcDlg(s)">编辑</a-button>
                  <a-button v-if="!s.builtin" size="small" danger @click="delSource(s)">删除</a-button>
                </div>
              </article>
            </div>
            <WmEmptyState v-else compact title="暂无仓库源" description="添加仓库源后，清单中的软件会出现在「应用商店」。" />
            <WmNotice tone="info" class="wm-mt-4">
              清单格式：JSON 裸数组或 {"apps":[...]}；应用声明支持 script（bash 内容或 http(s) 脚本地址）/ native（二进制发行包）/ compose 三类，字段与示例见仓库 docs/catalog.example.json。同名 key 按源顺序先注册者优先。
            </WmNotice>
          </div>
        </section>
      </a-tab-pane>

      <!-- ============ Tab 4 运行环境 ============ -->
      <a-tab-pane key="runtime">
        <template #tab>
          <span class="ops-tab"><CodeOutlined /><span class="ops-tab__label">运行环境</span><span class="ops-tab__count">{{ runtimes.length }}</span></span>
        </template>
        <section class="ops-panel">
          <header class="ops-panel__head">
            <div>
              <h3 class="ops-panel__title">运行时环境</h3>
              <p class="ops-panel__desc">编程语言多版本共存，安装后可切换默认版本并软链到系统路径。</p>
            </div>
            <div class="ops-panel__actions">
              <WmStatusPill :label="`${filteredRuntimes.length} 个运行时`" tone="neutral" :dot="false" outline />
            </div>
          </header>
          <div class="ops-panel__body">
            <div class="ops-card-grid" v-if="filteredRuntimes.length">
              <article v-for="r in filteredRuntimes" :key="r.key" class="store-card store-card--compact">
                <div class="store-card__head">
                  <span class="store-card__avatar store-card__avatar--runtime">{{ (r.name || '?').slice(0, 1) }}</span>
                  <div class="store-card__title">
                    <b class="wm-truncate">{{ r.name }}</b>
                    <WmStatusPill :label="`${r.installed.length} 个版本`" tone="info" :dot="false" />
                  </div>
                </div>
                <p class="store-card__desc">{{ r.desc }}</p>
                <div class="rt-chips" v-if="r.installed.length">
                  <a-tag v-for="i in r.installed" :key="i.id" :color="i.is_default ? 'success' : 'processing'">{{ i.version }}{{ i.is_default ? '（默认）' : '' }}</a-tag>
                </div>
                <div class="store-card__foot">
                  <a-button size="small" @click="openRt(r)">安装 / 管理版本</a-button>
                </div>
              </article>
            </div>
            <WmEmptyState v-else compact title="没有匹配的运行时" description="更换关键词后再试。" />
          </div>
        </section>
      </a-tab-pane>

      <!-- ============ Tab 5 已安装 ============ -->
      <a-tab-pane key="installed">
        <template #tab>
          <span class="ops-tab"><DownloadOutlined /><span class="ops-tab__label">已安装</span><span class="ops-tab__count">{{ apps.length }}</span></span>
        </template>
        <div class="ops-stack">
          <WmSectionCard title="实例管理" description="启动、停止、重启、升级、查看日志或卸载已安装软件" flush>
            <template #actions>
              <WmStatusPill :label="`${apps.length} 个实例`" tone="neutral" :dot="false" outline />
            </template>
            <WmTable :data="apps" v-loading="loading" empty-title="还没有安装软件" empty-description="从「应用商店」选择软件，选择下载源即可安装。">
              <WmColumn label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></WmColumn>
              <WmColumn label="软件" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></WmColumn>
              <WmColumn label="形态" width="90">
                <template #default="{ row }"><WmStatusPill :label="kindLabel(row.app.kind)" :tone="row.app.kind === 'package' ? 'success' : row.app.kind === 'native' ? 'warning' : 'info'" :dot="false" outline /></template>
              </WmColumn>
              <WmColumn label="状态" min-width="200"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></WmColumn>
              <WmColumn label="安装时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.app.created_at) }}</span></template></WmColumn>
              <WmColumn label="操作" width="220" fixed="right">
                <template #default="{ row }">
                  <a-dropdown v-if="!row.app.tool">
                    <a-button type="link" size="small">操作<span class="wm-icon wm-dd-icon"><ArrowDown /></span></a-button>
                    <template #overlay>
                      <a-menu>
                        <a-menu-item key="start" @click="act(row, 'start')">启动</a-menu-item>
                        <a-menu-item key="stop" @click="act(row, 'stop')">停止</a-menu-item>
                        <a-menu-item key="restart" @click="act(row, 'restart')">重启</a-menu-item>
                      </a-menu>
                    </template>
                  </a-dropdown>
                  <a-button type="link" size="small" @click="openUpgrade(row)">升级</a-button>
                  <a-button type="link" size="small" @click="viewLogs(row)">日志</a-button>
                  <a-button type="link" size="small" danger @click="uninstall(row)">卸载</a-button>
                </template>
              </WmColumn>
            </WmTable>
          </WmSectionCard>

          <div class="ops-split">
            <WmSectionCard title="下载源" description="安装时可为发行包选择来源">
              <div class="ops-list-row" v-for="item in sources" :key="item.id || item.name">
                <div class="ops-list-row__meta">
                  <b>{{ item.name }}</b>
                  <span>{{ item.id || '下载通道' }}</span>
                </div>
                <WmStatusPill label="可用" tone="success" :dot="false" outline />
              </div>
              <WmEmptyState v-if="!sources.length" compact title="暂无下载源" />
            </WmSectionCard>
            <WmSectionCard title="来源策略" description="安装器如何选择下载通道">
              <div class="ops-stack">
                <WmNotice tone="neutral">镜像站仅收录部分软件，未收录的源会在安装界面置灰；GitHub 类发布走加速通道。</WmNotice>
                <WmNotice tone="info">安装前可以先确认版本与来源，安装完成后可在实例管理中升级或查看运行状态。</WmNotice>
              </div>
            </WmSectionCard>
          </div>
        </div>
      </a-tab-pane>
    </a-tabs>

    <!-- 安装抽屉 -->
    <a-drawer v-model:open="dlg" :title="`安装 ${cur?.name || ''}`" :width="680">
      <a-form layout="vertical">
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">基本信息</div>
          <a-form-item label="实例名称"><a-input v-model:value="instName" placeholder="如 prod-node" /></a-form-item>
          <a-form-item v-if="cur?.versions?.length" label="版本" style="margin-bottom: 0">
            <a-select v-model:value="instVer" style="width: 100%">
              <a-select-option v-for="v in cur.versions" :key="v.version" :value="v.version">
                {{ v.version }}<template v-if="v.note">（{{ v.note }}）</template>
              </a-select-option>
            </a-select>
            <div class="hint">版本清单来自官方源和镜像目录，与面板运行在 Windows 还是 Linux 无关。</div>
          </a-form-item>
        </div>
        <div class="wm-form-sec" v-if="cur?.kind === 'native'">
          <div class="wm-form-sec__title">下载与安装</div>
          <a-form-item label="下载源" style="margin-bottom: 0">
            <a-select v-model:value="instSource" style="width: 100%">
              <a-select-option v-for="s in sources" :key="s.id"
                :value="s.id" :disabled="sourceDisabled(s.id)">{{ s.name }}</a-select-option>
            </a-select>
            <div v-if="sourceHint" class="hint">{{ sourceHint }}</div>
            <div class="hint">镜像站仅收录部分软件（未收录的源会置灰）；GitHub 类发布走加速通道</div>
          </a-form-item>
        </div>
        <div v-if="cur?.kind === 'package'" class="wm-form-sec">
          <div class="wm-form-sec__title">安装方式</div>
          <div class="hint">经系统镜像源安装（apt/dnf 当前配置的源；系统设置可一键更换国内镜像），由 systemd 托管服务。</div>
        </div>
        <div v-if="(cur?.params || []).length" class="wm-form-sec">
          <div class="wm-form-sec__title">参数</div>
          <a-form-item v-for="p in cur?.params || []" :key="p.key" :label="p.label" style="margin-bottom: 16px">
            <a-input v-model:value="instParams[p.key]" :placeholder="p.def" :type="p.key.includes('pass') || p.key === 'password' ? 'password' : 'text'" />
          </a-form-item>
        </div>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="deploying" @click="deploy">安装（下载发行包可能较慢）</a-button>
      </template>
    </a-drawer>

    <a-modal v-model:open="upDlg" :title="`升级 ${upRow?.name || ''}`" width="520px">
      <a-form layout="vertical">
        <a-form-item label="当前版本"><span class="wm-mono">{{ upRow?.app?.version || '未记录' }}</span></a-form-item>
        <a-form-item label="目标版本">
          <a-select v-model:value="upVer" style="width: 100%">
            <a-select-option v-for="v in upVersions" :key="v.version" :value="v.version">
              {{ v.version }}<template v-if="v.note">（{{ v.note }}）</template>
            </a-select-option>
          </a-select>
        </a-form-item>
      </a-form>
      <WmNotice tone="warning">升级会按所选版本重新安装并重启服务；仓库中不存在该版本时会失败。</WmNotice>
      <template #footer>
        <a-button @click="upDlg = false">取消</a-button>
        <a-button type="primary" :loading="upRunning" @click="doUpgrade">开始升级</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="logDlg" :title="`日志 - ${logName}`" width="860px">
      <WmCodeBlock :content="logContent" :label="`${logName} / logs`" />
    </a-modal>

    <!-- 上传/编辑脚本包 -->
    <a-modal v-model:open="scriptDlg" :title="scriptEditing ? `编辑脚本包 - ${scriptForm.name}` : '上传脚本包（bash）'" ok-text="保存" cancel-text="取消" :confirm-loading="scriptSaving" @ok="saveScript" width="760px">
      <a-form layout="vertical">
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">从仓库获取</div>
          <div class="script-fetch-row">
            <a-input v-model:value="scriptForm.fetchUrl" placeholder="脚本 raw 地址，如 https://cnb.cool/xxx/-/raw/main/scripts/foo.sh" @pressEnter="fetchScript" />
            <a-button :loading="scriptFetching" @click="fetchScript">获取内容</a-button>
          </div>
          <div class="hint">从 git 仓库 raw 链接拉取脚本内容自动填入下方（由面板服务器代理请求，绕过浏览器跨域）。</div>
        </div>
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">元数据</div>
          <div class="script-meta-grid">
            <a-form-item label="脚本名（key）" required>
              <a-input v-model:value="scriptForm.name" :disabled="scriptEditing" placeholder="如 init-blog（小写字母数字_-）" />
            </a-form-item>
            <a-form-item label="显示名"><a-input v-model:value="scriptForm.title" placeholder="默认与脚本名一致" /></a-form-item>
            <a-form-item label="版本"><a-input v-model:value="scriptForm.version" placeholder="1.0.0" /></a-form-item>
            <a-form-item label="分类"><a-input v-model:value="scriptForm.category" placeholder="如 系统工具 / 网站" /></a-form-item>
            <a-form-item label="图标（emoji 或单字）"><a-input v-model:value="scriptForm.icon" placeholder="如 🚀" style="width: 100%" /></a-form-item>
          </div>
          <a-form-item label="描述"><a-input v-model:value="scriptForm.desc" placeholder="脚本用途说明，显示在商店卡片" /></a-form-item>
        </div>
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">
            参数声明
            <a-button type="link" size="small" @click="scriptForm.params.push({ key: '', label: '', def: '' })">+ 添加参数</a-button>
          </div>
          <div v-for="(p, idx) in scriptForm.params" :key="idx" class="script-param-row">
            <a-input v-model:value="p.key" placeholder="key（如 db_pass）" style="width: 180px" />
            <a-input v-model:value="p.label" placeholder="表单标签" style="width: 220px" />
            <a-input v-model:value="p.def" placeholder="默认值" style="width: 160px" />
            <a-button type="link" danger size="small" @click="scriptForm.params.splice(idx, 1)">移除</a-button>
          </div>
          <div class="hint">安装时按声明生成表单；值以「双花括号包裹 key」占位符替换进脚本内容。</div>
        </div>
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">脚本内容</div>
          <a-form-item style="margin-bottom: 0">
            <a-textarea v-model:value="scriptForm.content" :rows="12" placeholder="#!/bin/bash&#10;echo hello" class="wm-mono" />
          </a-form-item>
        </div>
        <WmNotice tone="warning" class="wm-mt-4">脚本将以 bash 在面板服务器执行；保存后即出现在「应用商店」（来源=本地脚本），可带参数一键安装。</WmNotice>
      </a-form>
    </a-modal>

    <!-- 脚本日志 -->
    <a-modal v-model:open="scriptLogDlg" :title="`脚本日志 - ${scriptLogName}`" width="860px">
      <WmCodeBlock :content="scriptLogContent" :label="`${scriptLogName} / script log`" />
    </a-modal>

    <!-- 添加/编辑仓库源 -->
    <a-modal v-model:open="srcDlg" :title="srcEditing ? `编辑仓库源 - ${srcForm.name}` : '添加仓库源'" ok-text="保存" cancel-text="取消" :confirm-loading="srcSaving" @ok="saveSource" width="640px">
      <a-form layout="vertical">
        <a-form-item label="源名称" required><a-input v-model:value="srcForm.name" placeholder="如 我的应用仓库" /></a-form-item>
        <a-form-item label="仓库地址或清单 URL" required>
          <a-input v-model:value="srcForm.url" placeholder="https://cnb.cool/xxx/yyy（git 仓库，自动发现 catalog.json）或完整清单 URL" />
          <div class="hint">可直接填 git 仓库地址：面板按 CNB/GitLab/GitHub 的 raw 路径约定自动发现 catalog.json；也可填完整清单 URL。保存时立即拉取校验，应用声明支持 script/native/compose 三类。</div>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 运行时抽屉 -->
    <a-drawer v-model:open="rtDlg" width="860px" :title="`运行时版本 - ${rtLang?.name || ''}`">
      <WmToolbar>
        <a-select v-model:value="rtSource" style="width: 190px" @change="loadVersions">
          <a-select-option v-for="s in rtLang?.sources || []" :key="s.id" :value="s.id">{{ s.name }}</a-select-option>
        </a-select>
        <a-select v-model:value="rtFilterMode" style="width: 150px">
          <a-select-option value="common">显示常用版本</a-select-option>
          <a-select-option value="lts">只显示LTS版本</a-select-option>
          <a-select-option value="all">显示所有版本</a-select-option>
        </a-select>
        <a-input v-model:value="rtFilter" placeholder="版本过滤" style="width: 140px" allow-clear />
        <a-button size="small" @click="loadVersions">更新版本列表</a-button>
        <template v-if="rtLang?.key === 'nodejs'">
          <span class="hint">registry源</span>
          <a-select v-model:value="npmRegistry" placeholder="未设置" style="width: 200px" :loading="regLoading" @change="saveRegistry">
            <a-select-option v-for="o in registryOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
        </template>
        <template v-if="rtLang?.key === 'python'">
          <span class="hint">pypi源</span>
          <a-select v-model:value="pypiIndex" placeholder="未设置" style="width: 240px" :loading="pypiLoading" @change="savePypi">
            <a-select-option v-for="o in pypiOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
        </template>
        <template v-if="rtLang?.key === 'php'">
          <span class="hint">Composer源</span>
          <a-select v-model:value="composerUrl" placeholder="未设置" style="width: 210px" :loading="composerLoading" @change="saveComposer">
            <a-select-option v-for="o in composerMirrorOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
          <a-button v-if="!composerInstalled" size="small" type="primary" :loading="composerInstalling" @click="installComposer">安装 Composer</a-button>
          <span class="hint">PECL源</span>
          <a-select v-model:value="peclMirror" placeholder="未设置" style="width: 190px" @change="savePecl">
            <a-select-option v-for="o in peclOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
        </template>
        <template v-if="rtInstalled.length && rtLang?.key === 'nodejs'">
          <span class="hint">命令行版本</span>
          <a-select v-model:value="defaultInstId" style="width: 160px" @change="setDefaultById">
            <a-select-option v-for="i in rtInstalled" :key="i.id" :value="i.id">{{ i.version }}</a-select-option>
          </a-select>
        </template>
      </WmToolbar>
      <WmTable :data="filteredVersions" v-loading="rtLoading" size="small" max-height="380">
        <WmColumn label="版本" width="150">
          <template #default="{ row }"><b>{{ row.version }}</b> <a-tag v-if="row.lts" color="success">LTS</a-tag></template>
        </WmColumn>
        <WmColumn v-if="rtLang?.key === 'nodejs'" label="NPM" width="90"><template #default="{ row }">{{ row.npm || '-' }}</template></WmColumn>
        <WmColumn v-if="rtLang?.key === 'nodejs'" label="V8" width="110"><template #default="{ row }">{{ row.v8 || '-' }}</template></WmColumn>
        <WmColumn label="发布日期" width="120"><template #default="{ row }">{{ row.date || '-' }}</template></WmColumn>
        <WmColumn label="操作" width="110">
          <template #default="{ row }"><a-button size="small" type="primary" :loading="installingVer === row.version" @click="installVer(row)">安装</a-button></template>
        </WmColumn>
      </WmTable>
      <h4 class="store-subhead">已安装</h4>
      <WmTable :data="rtInstalled" size="small">
        <WmColumn prop="version" label="版本" width="150" />
        <WmColumn label="默认" width="70"><template #default="{ row }"><a-tag v-if="row.is_default" color="success">默认</a-tag><span v-else>-</span></template></WmColumn>
        <WmColumn label="软件路径" min-width="220"><template #default="{ row }"><span class="mono">{{ row.dir }}</span></template></WmColumn>
        <WmColumn label="安装时间" width="165"><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></WmColumn>
        <WmColumn label="操作" width="150">
          <template #default="{ row }">
            <a-button type="link" size="small" :disabled="row.is_default || rtLang?.key === 'php'" @click="setDefault(row)">设为默认</a-button>
            <a-button type="link" size="small" danger @click="rmRuntime(row)">卸载</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import {
  AppstoreOutlined,
  CloudOutlined,
  CodeOutlined,
  DownloadOutlined,
  FileTextOutlined,
  ReloadOutlined,
  SearchOutlined
} from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import { ArrowDown } from '../antd-compat'
import api from '../api'
import { fmtTime } from '../utils'

const storeTab = ref('apps')
const keyword = ref('')
const loading = ref(false)

const kindLabel = (k: string) => ({ native: '二进制', package: '系统包', compose: '容器', script: '脚本' } as any)[k] || k
const sourceLabel = (t: any) => (t.source === 'builtin' ? '内置' : t.source === 'local' ? '本地脚本' : t.source_name || '仓库')

// ---- 商店目录 ----
const templates = ref<any[]>([])
const catFilter = ref('all')
const sourceFilter = ref<'all' | 'builtin' | 'remote' | 'local'>('all')
const builtinCount = computed(() => templates.value.filter((t) => t.source === 'builtin').length)
const remoteCount = computed(() => templates.value.filter((t) => t.source === 'remote').length)
const localCount = computed(() => templates.value.filter((t) => t.source === 'local').length)
const catOptions = computed(() => {
  const cats = new Set<string>()
  for (const t of templates.value) if (t.category) cats.add(t.category)
  return [{ value: 'all', label: '全部分类' }, ...[...cats].sort().map((c) => ({ value: c, label: c }))]
})
const filteredTemplates = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return templates.value.filter((t) => {
    if (sourceFilter.value !== 'all' && t.source !== sourceFilter.value) return false
    if (catFilter.value !== 'all' && t.category !== catFilter.value) return false
    if (!q) return true
    return [t.name, t.key, t.desc, t.category, kindLabel(t.kind)].some((v: any) => String(v || '').toLowerCase().includes(q))
  })
})

// ---- 脚本包 ----
const customScripts = ref<any[]>([])
const scriptDlg = ref(false)
const scriptEditing = ref(false)
const scriptSaving = ref(false)
const scriptForm = reactive({ name: '', title: '', desc: '', version: '', category: '', icon: '', params: [] as any[], content: '', fetchUrl: '' })
const scriptFetching = ref(false)
const scriptLogDlg = ref(false)
const scriptLogName = ref('')
const scriptLogContent = ref('')

function openScriptDlg() {
  scriptEditing.value = false
  Object.assign(scriptForm, { name: '', title: '', desc: '', version: '', category: '', icon: '', content: '', fetchUrl: '' })
  scriptForm.params = []
  scriptDlg.value = true
}

async function fetchScript() {
  const u = scriptForm.fetchUrl.trim()
  if (!/^https?:\/\//.test(u)) {
    ElMessage.warning('请填写 http(s) 脚本地址')
    return
  }
  scriptFetching.value = true
  try {
    const d: any = await api.post('/apps/scripts/fetch', { url: u })
    scriptForm.content = d.content || ''
    // 脚本名为空时按 URL 文件名自动填充
    if (!scriptForm.name.trim()) {
      const base = u.split('?')[0].split('#')[0].split('/').pop() || ''
      if (base.endsWith('.sh')) scriptForm.name = base.slice(0, -3).toLowerCase().replace(/[^a-z0-9_-]/g, '-')
    }
    ElMessage.success('脚本内容已获取')
  } catch {
    /* 拉取失败提示由全局错误处理展示 */
  } finally {
    scriptFetching.value = false
  }
}

async function editScript(name: string) {
  try {
    const d: any = await api.get(`/apps/scripts/${name}`)
    scriptEditing.value = true
    Object.assign(scriptForm, { name: d.name, title: d.title, desc: d.desc, version: d.version, category: d.category, icon: d.icon || '', content: d.content })
    scriptForm.params = (d.params || []).map((p: any) => ({ ...p }))
    scriptDlg.value = true
  } catch {
    ElMessage.warning('脚本读取失败')
  }
}

async function saveScript() {
  const name = scriptForm.name.trim()
  if (!/^[a-z0-9][a-z0-9_-]{1,62}$/.test(name)) {
    ElMessage.warning('脚本名仅允许小写字母数字_-（2-64 位）')
    return
  }
  if (!scriptForm.content.trim()) {
    ElMessage.warning('脚本内容不能为空')
    return
  }
  for (const p of scriptForm.params) {
    if (p.key && !/^[a-z][a-z0-9_]{0,31}$/.test(p.key)) {
      ElMessage.warning(`参数 key 非法：${p.key}（小写字母开头，字母数字下划线）`)
      return
    }
  }
  scriptSaving.value = true
  try {
    await api.post('/apps/scripts', {
      name, title: scriptForm.title, desc: scriptForm.desc, version: scriptForm.version,
      category: scriptForm.category, icon: scriptForm.icon, params: scriptForm.params.filter((p) => p.key), content: scriptForm.content
    })
    ElMessage.success('脚本包已保存，可在「应用商店」安装')
    scriptDlg.value = false
    loadAll()
  } finally {
    scriptSaving.value = false
  }
}

async function delScript(name: string) {
  try {
    await ElMessageBox.confirm(`确认删除脚本包「${name}」？`, '删除', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/apps/scripts/${name}`)
  ElMessage.success('已删除')
  loadAll()
}

async function viewScriptLog(name: string) {
  try {
    const d: any = await api.get(`/apps/scripts/${name}/log`)
    scriptLogName.value = name
    scriptLogContent.value = d.log || '（暂无日志）'
    scriptLogDlg.value = true
  } catch {
    ElMessage.warning('暂无日志')
  }
}

// ---- 仓库源 ----
const appSources = ref<any[]>([])
const srcDlg = ref(false)
const srcEditing = ref(false)
const srcSaving = ref(false)
const srcForm = reactive({ id: '', name: '', url: '' })
const refreshingId = ref('')

function openSrcDlg(s?: any) {
  srcEditing.value = !!s
  Object.assign(srcForm, { id: s?.id || '', name: s?.name || '', url: s?.url || '' })
  srcDlg.value = true
}

async function saveSource() {
  if (!srcForm.name.trim() || !srcForm.url.trim()) {
    ElMessage.warning('请填写源名称与清单地址')
    return
  }
  srcSaving.value = true
  try {
    if (srcEditing.value) {
      await api.put(`/apps/sources/${srcForm.id}`, { name: srcForm.name, url: srcForm.url })
      ElMessage.success('仓库源已更新')
    } else {
      await api.post('/apps/sources', { name: srcForm.name, url: srcForm.url })
      ElMessage.success('仓库源已添加')
    }
    srcDlg.value = false
    loadAll()
  } finally {
    srcSaving.value = false
  }
}

async function toggleSource(s: any, enabled: boolean) {
  await api.put(`/apps/sources/${s.id}`, { enabled })
  s.enabled = enabled
  loadAll()
}

async function refreshSource(s: any) {
  refreshingId.value = s.id
  try {
    await api.post(`/apps/sources/${s.id}/refresh`)
    ElMessage.success('已刷新')
    loadAll()
  } finally {
    refreshingId.value = ''
  }
}

async function delSource(s: any) {
  try {
    await ElMessageBox.confirm(`确认删除仓库源「${s.name}」？其应用将从商店移除（已安装实例不受影响）。`, '删除', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/apps/sources/${s.id}`)
  ElMessage.success('已删除')
  loadAll()
}

// ---- 安装/实例管理（沿用安装引擎）----
const templatesRaw = ref<any[]>([])
const apps = ref<any[]>([])
const sources = ref<any[]>([])
const dlg = ref(false)
const deploying = ref(false)
const cur = ref<any>(null)
const instName = ref('')
const instSource = ref('auto')
const instVer = ref('')
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

const filteredRuntimes = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return runtimes.value
  return runtimes.value.filter((r) => [r.name, r.key, r.desc].some((v: any) => String(v || '').toLowerCase().includes(q)))
})

async function load() {
  loading.value = true
  try {
    templatesRaw.value = (await api.get('/apps/templates')) as any
    templates.value = templatesRaw.value.filter((t: any) => t.kind !== 'compose')
    apps.value = ((await api.get('/apps')) as any).filter((a: any) => a.app.kind !== 'compose')
    sources.value = (await api.get('/apps/soft-sources')) as any
  } finally {
    loading.value = false
  }
}

async function loadAll() {
  load()
  loadScripts()
  loadSourcesList()
  loadRuntimes()
}

async function loadScripts() {
  try {
    customScripts.value = (await api.get('/apps/scripts')) as any
  } catch {
    customScripts.value = []
  }
}

async function loadSourcesList() {
  try {
    appSources.value = (await api.get('/apps/sources')) as any
  } catch {
    appSources.value = []
  }
}

const upDlg = ref(false)
const upRow = ref<any>(null)
const upVer = ref('')
const upVersions = ref<any[]>([])
const upRunning = ref(false)

async function openUpgrade(row: any) {
  upRow.value = row
  upVer.value = ''
  upVersions.value = []
  upDlg.value = true
  try {
    const d: any = await api.get('/apps/versions', { params: { key: row.app.app_key } })
    upVersions.value = d.versions || []
    if (d.default && !upVer.value) upVer.value = d.default
  } catch { /* 清单拉取失败时保持空 */ }
}

async function doUpgrade() {
  if (!upVer.value) {
    ElMessage.warning('请选择目标版本')
    return
  }
  upRunning.value = true
  try {
    await api.post(`/apps/${upRow.value.app.id}/upgrade`, { version: upVer.value }, { timeout: 900000 })
    ElMessage.success('已升级到 ' + upVer.value)
    upDlg.value = false
    load()
  } finally {
    upRunning.value = false
  }
}

async function openInstall(t: any) {
  cur.value = t
  instName.value = String(t.key || '').replace('custom:', '')
  instSource.value = 'auto'
  instVer.value = t.kind === 'package' ? '' : (t.default_version || '')
  Object.keys(instParams).forEach((k) => delete instParams[k])
  for (const p of t.params || []) instParams[p.key] = p.def || ''
  dlg.value = true
  // 仅 native 拉取可选版本清单（package 统一走系统镜像源，不选版本）
  if (t.kind === 'native') {
    try {
      const d: any = await api.get('/apps/versions', { params: { key: t.key } })
      cur.value = { ...t, versions: d.versions || [] }
      if (d.default && !instVer.value) instVer.value = d.default
    } catch { /* 拉取失败时无版本选择 */ }
  }
}

function sourceDisabled(id: string) {
  if (!cur.value) return false
  if (cur.value.soft_sources && !cur.value.soft_sources.includes(id)) return true
  // nginx.org 与 packages.redis.io 的历史版本没有对应的通用 apt 镜像，避免提交后才失败。
  if (cur.value.key === 'nginx-pkg' && instVer.value && instVer.value !== 'distro') return !['auto', 'official'].includes(id)
  if (cur.value.key === 'redis-pkg' && instVer.value && instVer.value !== 'distro') return !['auto', 'official'].includes(id)
  if (cur.value.key === 'mariadb' && instVer.value && instVer.value !== 'distro') return !['auto', 'official', 'aliyun'].includes(id)
  if (cur.value.key === 'postgresql-pkg' && instVer.value && instVer.value !== 'distro') return !['auto', 'official', 'aliyun', 'tencent'].includes(id)
  return false
}

async function deploy() {
  if (!instName.value) {
    ElMessage.warning('请填写实例名称')
    return
  }
  deploying.value = true
  try {
    // package 统一走系统镜像源（后端强制，版本/源参数不受理）
    const payload = cur.value.kind === 'package'
      ? { app_key: cur.value.key, name: instName.value, params: { ...instParams } }
      : { app_key: cur.value.key, name: instName.value, version: instVer.value, mirror: instSource.value, params: { ...instParams } }
    const d: any = await api.post('/apps', payload, { timeout: 1900000 })
    ElMessage.success(d?.status || '安装完成')
    dlg.value = false
    loadAll()
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
    await ElMessageBox.confirm(`确认卸载「${row.app.name}」？`, '卸载', { type: 'warning' })
  } catch {
    return
  }
  try {
    await ElMessageBox.confirm('是否同时删除安装目录（不可恢复）？', '删除文件', {
      type: 'error', confirmButtonText: '删除文件', cancelButtonText: '保留文件'
    })
    removeData = true
  } catch {
    removeData = false
  }
  // 面板依赖保护：nginx 等被面板反代/网站占用的系统包需二次确认强卸
  let force = false
  try {
    await api.post(`/apps/${row.app.id}/uninstall`, { remove_data: removeData, force })
  } catch (e: any) {
    if (!e?.msg?.includes('面板正在使用')) throw e
    try {
      await ElMessageBox.confirm(e.msg, '面板依赖警告', { type: 'error', confirmButtonText: '仍要强制卸载', cancelButtonText: '取消' })
    } catch {
      return
    }
    force = true
    await api.post(`/apps/${row.app.id}/uninstall`, { remove_data: removeData, force })
  }
  ElMessage.success('已卸载')
  loadAll()
}

// ---- 运行环境（功能保留）----
const runtimes = ref<any[]>([])
const rtDlg = ref(false)
const rtLang = ref<any>(null)
const rtSource = ref('')
const rtVersions = ref<any[]>([])
const rtFilter = ref('')
const rtLoading = ref(false)
const installingVer = ref('')
const rtFilterMode = ref('common')
const npmRegistry = ref('')
const regLoading = ref(false)
const registryOptions = [
  { id: 'https://registry.npmmirror.com', name: 'npmmirror 中国镜像' },
  { id: 'https://registry.npmjs.org', name: 'npm 官方' },
  { id: 'https://mirrors.cloud.tencent.com/npm/', name: '腾讯云镜像' },
  { id: 'https://mirrors.huaweicloud.com/repository/npm/', name: '华为云镜像' }
]
const pypiOptions = [
  { id: 'https://pypi.tuna.tsinghua.edu.cn/simple/', name: '清华大学镜像（社区首选）' },
  { id: 'https://mirrors.aliyun.com/pypi/simple/', name: '阿里云镜像' },
  { id: 'https://pypi.mirrors.ustc.edu.cn/simple/', name: '中科大镜像' },
  { id: 'https://repo.huaweicloud.com/repository/pypi/simple/', name: '华为云镜像' },
  { id: 'https://mirrors.cloud.tencent.com/pypi/simple/', name: '腾讯云镜像' },
  { id: 'https://pypi.doubanio.com/simple/', name: '豆瓣镜像' },
  { id: 'https://pypi.org/simple/', name: 'PyPI 官方' }
]
const pypiIndex = ref('')
const pypiLoading = ref(false)
const composerUrl = ref('')
const composerLoading = ref(false)
const composerInstalling = ref(false)
const composerInstalled = ref(false)
const peclMirror = ref('')
const composerMirrorOptions = [
  { id: 'https://mirrors.aliyun.com/composer/', name: '阿里云镜像（首选）' },
  { id: 'https://mirrors.cloud.tencent.com/composer/', name: '腾讯云镜像' },
  { id: 'https://repo.huaweicloud.com/repository/php/', name: '华为云镜像' },
  { id: 'https://packagist.cn', name: 'Packagist CN' },
  { id: 'https://packagist.org', name: 'Packagist 官方' }
]
const peclOptions = [
  { id: 'https://pecl.php.net', name: 'PECL 官方' },
  { id: 'https://mirrors.aliyun.com/pecl/', name: '阿里云镜像' },
  { id: 'https://mirrors.tuna.tsinghua.edu.cn/pecl/', name: '清华 TUNA' }
]
const defaultInstId = computed(() => {
  const d = rtInstalled.value.find((i) => i.is_default)
  return d ? d.id : undefined
})
const rtInstalled = computed(() => rtLang.value?.installed || [])
const filteredVersions = computed(() => {
  let list = rtVersions.value
  if (rtFilterMode.value === 'lts') list = list.filter((v) => v.lts)
  else if (rtFilterMode.value === 'common') {
    const newest = list[0]?.version
    list = list.filter((v) => v.lts || v.version === newest)
  }
  const f = rtFilter.value.trim().toLowerCase()
  if (!f) return list
  return list.filter((v) => v.version.toLowerCase().includes(f))
})

async function loadRuntimes() {
  try {
    runtimes.value = (await api.get('/runtimes')) as any
  } catch {
    runtimes.value = []
  }
}

function openRt(r: any) {
  rtLang.value = r
  rtSource.value = r.sources?.[0]?.id || ''
  rtVersions.value = []
  rtFilter.value = ''
  rtFilterMode.value = r.key === 'nodejs' ? 'common' : 'all'
  npmRegistry.value = ''
  rtDlg.value = true
  loadVersions()
  if (r.key === 'nodejs') loadRegistry()
  if (r.key === 'python') loadPypi()
  if (r.key === 'php') { loadComposerInfo(); loadPecl() }
}

async function loadRegistry() {
  regLoading.value = true
  try {
    const d: any = await api.get('/runtimes/nodejs/registry')
    npmRegistry.value = d.registry || ''
  } finally {
    regLoading.value = false
  }
}

async function saveRegistry(val: string) {
  await api.post('/runtimes/nodejs/registry', { registry: val })
  ElMessage.success('npm registry 已设置')
}

async function loadPypi() {
  pypiLoading.value = true
  try {
    const d: any = await api.get('/runtimes/python/pypi')
    pypiIndex.value = d.index_url || ''
  } finally {
    pypiLoading.value = false
  }
}

async function savePypi(val: string) {
  await api.post('/runtimes/python/pypi', { index_url: val })
  ElMessage.success('pip 源已设置，pip install 将走该镜像')
}

async function setDefaultById(id: number) {
  await setDefault({ id })
}

async function loadVersions() {
  if (!rtLang.value) return
  rtLoading.value = true
  try {
    rtVersions.value = (await api.get(`/runtimes/${rtLang.value.key}/versions`, { params: { source: rtSource.value }, timeout: 60000 })) as any
  } finally {
    rtLoading.value = false
  }
}

async function installVer(row: any) {
  installingVer.value = row.version
  try {
    // PHP 经发行版仓库安装（含仓库配置与 apt-get update），服务端上限 30 分钟
    const timeout = rtLang.value.key === 'php' ? 1800000 : 900000
    const d: any = await api.post(`/runtimes/${rtLang.value.key}/install`, { version: row.version, source: rtSource.value }, { timeout })
    ElMessage.success('安装完成，软链: ' + (d.links || []).join(', '))
    loadRuntimes()
  } finally {
    installingVer.value = ''
  }
}

async function setDefault(row: any) {
  await api.post(`/runtimes/${rtLang.value.key}/set-default`, { instance_id: row.id })
  ElMessage.success('已设为默认')
  loadRuntimes()
  if (rtDlg.value && rtLang.value) {
    const cur = runtimes.value.find((r) => r.key === rtLang.value.key)
    if (cur) rtLang.value = cur
  }
}

async function rmRuntime(row: any) {
  try {
    await ElMessageBox.confirm(`确认卸载 ${rtLang.value?.name || ''} ${row.version}？对应软链将移除。`, '卸载运行时', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/runtimes/${row.id}`)
  ElMessage.success('已卸载')
  loadRuntimes()
}

async function loadComposerInfo() {
  composerLoading.value = true
  try {
    const d: any = await api.get('/runtimes/php/composer')
    composerInstalled.value = !!d.installed
    composerUrl.value = d.url || ''
  } finally {
    composerLoading.value = false
  }
}

async function saveComposer(val: string) {
  await api.post('/runtimes/php/composer', { url: val })
  ElMessage.success('Composer 源已设置')
}

async function installComposer() {
  composerInstalling.value = true
  try {
    await api.post('/runtimes/php/composer/install', {}, { timeout: 900000 })
    ElMessage.success('Composer 已安装')
    composerInstalled.value = true
  } finally {
    composerInstalling.value = false
  }
}

async function loadPecl() {
  try {
    const d: any = await api.get('/runtimes/php/pecl')
    peclMirror.value = d.mirror || ''
  } catch {
    peclMirror.value = ''
  }
}

async function savePecl(val: string) {
  await api.post('/runtimes/php/pecl', { mirror: val })
  ElMessage.success('PECL 镜像已设置')
}

onMounted(() => loadAll())
</script>

<style scoped>
.rt-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin: 0 0 var(--wm-s3);
}
.store-card__foot--wrap {
  flex-wrap: wrap;
  gap: 6px;
}
.store-card__avatar--local {
  background: color-mix(in srgb, var(--wm-primary, #0f766e) 14%, #fff);
  color: var(--wm-primary, #0f766e);
}
.script-meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: 12px;
}
.script-param-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
.script-fetch-row {
  display: flex;
  gap: 8px;
}
.store-src-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.store-src {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid var(--wm-border, #e4e7ec);
  border-radius: 12px;
  background: var(--wm-surface, #fff);
}
.store-src--off {
  opacity: 0.62;
}
.store-src__title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.store-src__url {
  font-size: 12px;
  margin: 0 0 4px;
  max-width: 560px;
}
.store-src__meta {
  font-size: 12px;
  color: var(--wm-text-secondary, #667085);
  margin: 0;
}
.store-src__err {
  color: var(--wm-danger, #d92d20);
}
.store-src__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
</style>
