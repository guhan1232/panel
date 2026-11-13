<template>
  <div class="wm-page setting-page">
    <div class="setting-shell">
      <aside class="setting-nav">
        <div class="setting-nav__title">系统设置</div>
        <a v-for="g in groups" :key="g.id" :href="'#' + g.id" class="setting-nav__item" :class="{ 'setting-nav__item--active': activeGroup === g.id }" @click.prevent="jumpTo(g.id)">
          <span class="setting-nav__dot"></span>{{ g.label }}
        </a>
      </aside>
      <div class="setting-main">
      <section id="sec-account" class="setting-group">
        <div class="setting-group__head"><span class="setting-group__bar"></span><h3>账号与安全</h3></div>
      <WmSectionCard title="账号信息" description="修改登录用户名；初始随机用户名建议尽快改为易记名称">
        <a-form layout="vertical" style="max-width: 440px">
          <a-form-item label="当前用户名"><span class="wm-mono"><b>{{ auth.username }}</b></span></a-form-item>
          <a-form-item label="新用户名" style="margin-bottom: 0"><a-input v-model:value="newUsername" placeholder="3-64 位，字母/数字/_/-" /></a-form-item>
        </a-form>
        <WmNotice tone="neutral" class="wm-mt-4">修改后需用新用户名重新登录；SSH 首次输出的初始凭据文件将在你首次修改密码后自动删除。</WmNotice>
        <template #footer>
          <a-button type="primary" :loading="savingName" @click="saveUsername">保存用户名</a-button>
        </template>
      </WmSectionCard>
      <WmSectionCard title="修改管理员密码" description="新密码至少 8 位，修改后当前会话保持有效" marker dense>
        <a-form layout="vertical" style="max-width: 440px">
          <a-form-item label="原密码"><a-input-password v-model:value="form.old" /></a-form-item>
          <a-form-item label="新密码"><a-input-password v-model:value="form.n1" placeholder="至少 8 位" /></a-form-item>
          <a-form-item label="确认密码" style="margin-bottom: 0"><a-input-password v-model:value="form.n2" /></a-form-item>
        </a-form>
        <template #footer>
          <a-button type="primary" :loading="saving" @click="submit">修改密码</a-button>
        </template>
      </WmSectionCard>

      <WmSectionCard title="两因素验证（TOTP）" description="启用后登录需输入验证器 App 的 6 位动态码">
        <template #actions>
          <a-button size="small" type="primary" @click="setup2fa">开始绑定</a-button>
          <a-button size="small" data-tone="danger" @click="disable2fa">解除绑定</a-button>
        </template>
        <div v-if="faSecret">
          <p class="wm-text-muted">将以下密钥录入验证器（Google Authenticator / 微信小程序「腾讯 Token」等），然后输入动态码确认：</p>
          <p class="wm-mono"><b>{{ faSecret }}</b></p>
          <p class="hint wm-mono">{{ faURL }}</p>
          <a-input v-model:value="faCode" placeholder="6 位动态码" style="width: 220px" />
          <a-button type="primary" size="small" :loading="faSaving" @click="confirm2fa">确认启用</a-button>
        </div>
        <p v-else class="wm-text-muted">尚未绑定或已解除。</p>
      </WmSectionCard>
      <WmSectionCard title="在线会话" description="当前账号在其他设备的登录会话，可踢出">
        <template #actions>
          <a-button size="small" data-tone="danger" :loading="revokingOthers" :disabled="!sessions.some((x: any) => !x.current)" @click="revokeOthers">退出其他设备</a-button>
          <a-button size="small" @click="loadSessions">刷新</a-button>
        </template>
        <WmTable :data="sessions" size="small" empty-title="没有在线会话" empty-description="">
          <WmColumn label="IP" width="150"><template #default="{ row }"><span class="wm-mono">{{ row.ip }}</span></template></WmColumn>
          <WmColumn label="客户端" width="150">
            <template #default="{ row }">
              <WmStatusPill v-if="row.client === 'app'" :label="'APP' + (row.device_name ? ' · ' + row.device_name : '')" tone="success" :dot="false" />
              <WmStatusPill v-else label="Web" tone="info" :dot="false" />
            </template>
          </WmColumn>
          <WmColumn label="设备" min-width="220"><template #default="{ row }"><span class="wm-text-muted wm-truncate">{{ row.ua }}</span></template></WmColumn>
          <WmColumn label="过期" width="170"><template #default="{ row }"><span class="wm-mono">{{ row.expires }}</span></template></WmColumn>
          <WmColumn label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <a-button v-if="!row.current" type="link" size="small" danger @click="revoke(row)">踢出</a-button>
              <WmStatusPill v-else label="本设备" tone="info" :dot="false" />
            </template>
          </WmColumn>
        </WmTable>
      </WmSectionCard>
      </section>
      <section id="sec-access" class="setting-group">
        <div class="setting-group__head"><span class="setting-group__bar"></span><h3>接入与扩展</h3></div>
      <WmSectionCard title="开放 API 与 MCP" description="API Token 供外部脚本直调面板 API；MCP Server 供 AI 客户端（Trae / Claude / Cursor 等）管理面板" marker>
        <template #actions>
          <a-button size="small" @click="loadApiToken">刷新</a-button>
        </template>
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="状态">
            <WmStatusPill :label="apiToken.enabled ? '已启用' : '未启用'" :tone="apiToken.enabled ? 'success' : 'warning'" :dot="false" />
          </a-descriptions-item>
          <a-descriptions-item label="Token">
            <span class="wm-mono">{{ apiToken.token ? (tokenRevealed ? apiToken.token : apiToken.token.slice(0, 8) + '••••••••') : '—' }}</span>
            <a-button v-if="apiToken.token" size="small" type="link" @click="tokenRevealed = !tokenRevealed">{{ tokenRevealed ? '隐藏' : '显示' }}</a-button>
          </a-descriptions-item>
          <a-descriptions-item label="权限">
            <WmStatusPill :label="permLabel" :tone="apiToken.readonly ? 'warning' : 'neutral'" :dot="false" outline />
          </a-descriptions-item>
          <a-descriptions-item label="MCP 端点"><span class="wm-mono">{{ panelOrigin }}/api/mcp</span></a-descriptions-item>
        </a-descriptions>
        <div class="wm-mt-4">
          <a-space wrap>
            <a-button type="primary" :loading="tokenBusy" @click="genApiToken">{{ apiToken.enabled ? '重置 Token' : '生成 Token 启用' }}</a-button>
            <a-button v-if="apiToken.enabled" data-tone="danger" :loading="tokenBusy" @click="clearApiToken">禁用开放 API</a-button>
          </a-space>
        </div>
        <template v-if="apiToken.enabled">
          <div class="wm-mt-4" style="max-width: 720px">
            <a-form layout="vertical">
              <a-form-item label="权限模式">
                <a-radio-group v-model:value="permForm.mode">
                  <a-radio-button value="full">完整权限（等效管理员）</a-radio-button>
                  <a-radio-button value="custom">按模块授权</a-radio-button>
                </a-radio-group>
              </a-form-item>
              <template v-if="permForm.mode === 'custom'">
                <a-form-item>
                  <a-checkbox-group v-model:value="permForm.scopes" style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 6px 12px">
                    <a-checkbox v-for="s in apiToken.scopes_known || []" :key="s" :value="s">{{ scopeLabel(s) }}</a-checkbox>
                  </a-checkbox-group>
                </a-form-item>
              </template>
              <a-form-item style="margin-bottom: 0">
                <a-checkbox v-model:checked="permForm.readonly">只读模式（仅允许查询类请求，所有写操作将被拒绝）</a-checkbox>
              </a-form-item>
            </a-form>
          </div>
        </template>
        <WmCodeBlock v-if="apiToken.enabled && tokenRevealed" :content="mcpCfg" label="MCP 客户端配置（Trae / Claude Code / Cursor 通用）" class="wm-mt-4" />
        <WmNotice tone="warning" class="wm-mt-4">Token 等效管理员权限：仅经 HTTPS 调用，泄露后立即重置。REST 示例：curl -H "Authorization: Bearer &lt;token&gt;" {{ panelOrigin }}/api/sites</WmNotice>
        <template v-if="apiToken.enabled" #footer>
          <span class="wm-text-muted st-foot-note">保存立即生效，Token 值不变，客户端无需重新配置</span>
          <a-button size="small" type="primary" :loading="permSaving" @click="savePerm">保存权限</a-button>
        </template>
      </WmSectionCard>
      <WmSectionCard title="移动 APP" description="安卓 APP 连接与自更新：对外地址用于扫码登录二维码；APK 地址供 APP 检查更新与下载">
        <a-form layout="vertical" style="max-width: 560px">
          <a-form-item label="面板对外地址">
            <a-input v-model:value="appForm.public_url" placeholder="留空按反代头/请求头自动推导，如 https://panel.hanyuxin.cn" />
          </a-form-item>
          <a-form-item label="APK 下载地址">
            <a-input v-model:value="appForm.apk_url" placeholder="https://…/hanyuxin-panel.apk" />
          </a-form-item>
          <a-form-item label="APP 最新版本">
            <a-input v-model:value="appForm.latest_version" placeholder="如 1.0.0（APP 启动时比对提示更新）" />
          </a-form-item>
          <a-form-item label="更新说明" style="margin-bottom: 0">
            <a-textarea v-model:value="appForm.release_notes" :rows="3" placeholder="本版更新内容" />
          </a-form-item>
        </a-form>
        <template #footer>
          <a-button type="primary" size="small" :loading="appSaving" @click="saveAppConfig">保存 APP 配置</a-button>
        </template>
      </WmSectionCard>
      </section>
      <section id="sec-network" class="setting-group">
        <div class="setting-group__head"><span class="setting-group__bar"></span><h3>网络与证书</h3></div>
      <WmSectionCard title="HTTPS 加密连接" description="中间 CA 签发（推荐）/ Let's Encrypt 公共免费 CA（域名）/ 纯自签 10 年兜底" marker>
        <template #actions>
          <a-button size="small" @click="loadTls">刷新</a-button>
        </template>
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="当前模式">
            <WmStatusPill :label="tls.tls ? 'HTTPS 加密' : 'HTTP 明文'" :tone="tls.tls ? 'success' : 'warning'" />
          </a-descriptions-item>
          <a-descriptions-item label="证书主体"><span class="wm-mono">{{ tls.cert_subject || '—' }}</span></a-descriptions-item>
          <a-descriptions-item label="有效期至"><span class="wm-mono">{{ tls.cert_not_after || '—' }}</span></a-descriptions-item>
          <a-descriptions-item label="中间 CA">
            <WmStatusPill :label="tls.has_ca ? '已保存（可复用续签）' : '未上传'" :tone="tls.has_ca ? 'success' : 'default'" :dot="false" />
          </a-descriptions-item>
        </a-descriptions>
        <a-form layout="vertical" style="max-width: 560px" class="wm-mt-4">
          <a-form-item label="访问 IP/域名（写入证书 SAN，逗号分隔）" style="margin-bottom: 0"><a-input v-model:value="extraIps" placeholder="如 203.0.113.10, panel.example.com" /></a-form-item>
        </a-form>
        <p class="wm-text-muted" style="margin-top:8px">CA 上传一次后保存于服务器（data/certs/ca），续签无需重复上传。云服务器 NAT 下公网 IP 不在网卡上，请在上方显式填写访问 IP 以保证证书 SAN 匹配。应用后面板自动重启为 HTTPS。</p>
        <WmNotice tone="info" class="wm-mt-4">仓库一键安装脚本 scripts/install.sh 同样支持：--ca-cert/--ca-key 用中间 CA 签发（有效期跟随 CA）；不给 CA 自动自签 10 年。</WmNotice>
        <template #footer>
          <a-button type="primary" :loading="tlsBusy === 'ca'" @click="openCaDlg">用中间 CA 签发（推荐 · 跟随 CA 有效期）</a-button>
          <a-button :loading="tlsBusy === 'acme'" @click="openAcmeDlg">Let's Encrypt 免费签发</a-button>
          <a-button :loading="tlsBusy === 'self'" @click="applyTls('self')">纯自签兜底（10 年）</a-button>
        </template>
      </WmSectionCard>
      <WmSectionCard title="域名反代访问（可选）" description="安装 NGINX 后可用自有域名反代面板并挂证书，域名 HTTPS 访问" marker>
        <template #actions>
          <a-button size="small" @click="loadProxy">刷新</a-button>
        </template>
        <!-- NGINX 状态 -->
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="NGINX">
            <WmStatusPill :label="nginx.installed ? (nginx.running ? '已安装 · 运行中' : '已安装 · 未运行') : '未安装'" :tone="nginx.installed ? 'success' : 'warning'" :dot="false" />
          </a-descriptions-item>
          <a-descriptions-item v-if="proxySite" label="反代站点">
            <span class="wm-mono">{{ proxySite.site?.domains }} → {{ proxySite.site?.upstream }}</span>
            <WmStatusPill :label="proxySite.site?.enabled ? '启用中' : '已停用'" :tone="proxySite.site?.enabled ? 'success' : 'default'" :dot="false" />
            <a-space :size="4" style="margin-left:12px">
              <a-switch :checked="proxySite.site?.enabled" size="small" @change="toggleProxy" />
              <a-button size="small" @click="viewProxyLog('access')">访问日志</a-button>
              <a-button size="small" @click="viewProxyLog('error')">错误日志</a-button>
            </a-space>
          </a-descriptions-item>
        </a-descriptions>
        <div v-if="!nginx.installed" class="wm-mt-4">
          <a-button size="small" type="primary" :loading="nginxInstalling" @click="installNginx">一键安装 NGINX</a-button>
        </div>
        <a-form layout="vertical" style="max-width: 560px" class="wm-mt-4">
          <a-form-item label="反代域名（需先把域名 A 记录解析到服务器公网 IP）">
            <a-input v-model:value="proxyForm.domain" placeholder="如 panel.example.com" />
          </a-form-item>
          <a-form-item label="证书（HTTPS 用）">
            <a-select v-model:value="proxyForm.certSel" style="width: 100%">
              <a-select-option value="">暂不启用 HTTPS（HTTP 反代）</a-select-option>
              <a-select-option v-if="tls.cert_path" value="panel">面板自用证书（{{ tls.cert_subject || '面板 CA' }}）</a-select-option>
              <a-select-option v-for="c in certList" :key="c.domain" :value="c.domain">{{ c.domain }}（{{ c.days_left }} 天，{{ c.source }}）</a-select-option>
            </a-select>
            <div class="hint">证书在「SSL 证书」页上传；面板自用证书即当前 HTTPS 所用的 CA 签发证书（SAN 为 IP，浏览器对域名会提示不匹配，可上传域名证书消除）。</div>
          </a-form-item>
          <a-form-item style="margin-bottom: 0">
            <a-checkbox v-model:checked="proxyForm.forceHttps">HTTP 自动跳转 HTTPS</a-checkbox>
          </a-form-item>
        </a-form>
        <WmNotice tone="neutral" class="wm-mt-4">反代由 NGINX 承载：域名 → NGINX(80/443) → 面板端口。反代配置含 WebSocket 支持（终端/实时数据可用）。启用后可用 https://域名/s/安全入口 访问面板。</WmNotice>
        <template #footer>
          <a-button type="primary" :loading="proxySaving" :disabled="!nginx.installed || !proxyForm.domain" @click="applyProxy">启用反代</a-button>
          <a-button v-if="proxySite" danger @click="removeProxy">移除反代</a-button>
        </template>
      </WmSectionCard>
      </section>
      <section id="sec-ops" class="setting-group">
        <div class="setting-group__head"><span class="setting-group__bar"></span><h3>系统运维</h3></div>
      <WmSectionCard title="系统软件源" description="一键把操作系统 apt/dnf 软件源换成国内镜像站并持久生效（自动备份原文件，可恢复）">
        <template #actions>
          <a-button size="small" @click="loadSysSrc">刷新</a-button>
        </template>
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="系统"><span class="wm-mono">{{ sysSrc.os_id }} {{ sysSrc.os_ver }}</span></a-descriptions-item>
          <a-descriptions-item label="包管理"><span class="wm-mono">{{ sysSrc.pkg }}</span></a-descriptions-item>
          <a-descriptions-item label="当前源"><span class="wm-mono">{{ sysSrc.current || '—' }}</span></a-descriptions-item>
          <a-descriptions-item label="原始备份">
            <WmStatusPill :label="sysSrc.backup ? '已保存（可恢复）' : '未更换过'" :tone="sysSrc.backup ? 'success' : 'default'" :dot="false" />
          </a-descriptions-item>
        </a-descriptions>
        <a-form layout="vertical" style="max-width: 560px" class="wm-mt-4">
          <a-form-item label="目标镜像站">
            <a-select v-model:value="sysTarget" style="width: 100%">
              <a-select-option v-for="t in sysSrc.targets || []" :key="t.id" :value="t.id">{{ t.name }}</a-select-option>
            </a-select>
            <div class="hint">改写系统源文件（sources.list / DEB822），保留 suite 与组件结构；换源后自动 apt-get update 验证。第三方源（Docker/MySQL/PHP 等 .d 仓库）不受影响。</div>
          </a-form-item>
        </a-form>
        <WmCodeBlock v-if="sysSrcOut" :content="sysSrcOut" label="update 输出尾部" class="wm-mt-4" />
        <template #footer>
          <a-button type="primary" size="small" :loading="sysSrcBusy" @click="applySysSrc">一键更换</a-button>
          <a-button size="small" :disabled="!sysSrc.backup" :loading="sysSrcBusy" @click="restoreSysSrc">恢复原始源</a-button>
        </template>
      </WmSectionCard>

      <WmSectionCard title="面板在线升级" description="从升级清单检查新版本，下载校验后原子替换并自动重启">
        <template #actions>
          <a-button size="small" @click="checkUpgrade">检查更新</a-button>
          <a-button v-if="upInfo.has_update" size="small" type="primary" :loading="upApplying" @click="applyUpgrade">升级到 {{ upInfo.remote }}</a-button>
        </template>
        <div v-if="upInfo.current">
          <p>当前版本 <b class="wm-mono">{{ upInfo.current }}</b>
            <template v-if="upInfo.remote"> · 清单版本 <b class="wm-mono">{{ upInfo.remote }}</b>
            <WmStatusPill v-if="upInfo.has_update" label="有更新" tone="warning" />
            <WmStatusPill v-else-if="upInfo.reachable" label="已是最新" tone="success" />
            </template></p>
          <p v-if="upInfo.notes" class="wm-text-muted">{{ upInfo.notes }}</p>
          <p v-if="upInfo.err" class="wm-text-muted">清单不可达：{{ upInfo.err }}（可在配置 upgrade_manifest_url 更换升级源）</p>
        </div>
        <WmNotice tone="warning" class="wm-mt-4">升级会自动备份当前二进制为 *.bak 并重启服务，重启期间面板短暂不可访问。</WmNotice>
      </WmSectionCard>
      <WmSectionCard title="面板数据备份 / 恢复" description="导出：打包数据库快照、config.yaml 与托管证书；导入：上传备份包一键恢复（自动留存回滚点）">
        <template #actions>
          <a-button size="small" type="primary" @click="downloadBackup">导出备份包</a-button>
          <a-button size="small" @click="restoreDlg = true; restoreFile = null">导入恢复</a-button>
          <a-button v-if="hasRollback" size="small" data-tone="danger" :loading="rollbackBusy" @click="doRollback">回滚上次恢复前数据</a-button>
        </template>
        <p class="wm-text-muted">导出包含 panel.db（一致性快照）、config.yaml（含安全入口/TLS）与 certs-managed 目录。导入恢复支持按项勾选（数据库 / 配置 / 证书），恢复前自动把当前数据留存为回滚点，面板自动重启；恢复配置项会改变端口/入口/TLS，请留意提示。</p>
      </WmSectionCard>
      </section>
      </div>
    </div>

    <a-modal v-model:open="acmeDlg" title="Let's Encrypt 免费签发（公共 CA）" ok-text="签发并启用 HTTPS" cancel-text="取消" @ok="acmeApply">
      <a-form layout="vertical">
        <a-form-item label="域名" required>
          <a-input v-model:value="acmeForm.domain" placeholder="如 panel.example.com（需已解析到本服务器公网 IP）" />
        </a-form-item>
        <a-form-item label="联系邮箱（可选，到期提醒用）">
          <a-input v-model:value="acmeForm.email" placeholder="admin@example.com" />
        </a-form-item>
        <WmNotice tone="warning">签发通过 HTTP-01 挑战：本服务器需占用 80 端口约 10 秒，且域名必须已解析到本服务器公网 IP。证书 90 天有效，面板自动续期。</WmNotice>
      </a-form>
    </a-modal>
    <!-- 反代日志 -->
    <a-modal v-model:open="proxyLogDlg" :title="proxyLogName" width="820px">
      <WmCodeBlock :content="proxyLogContent" :label="proxyLogName" />
    </a-modal>

    <!-- 面板备份导入恢复 -->
    <a-modal v-model:open="restoreDlg" title="导入备份包恢复面板" ok-text="开始恢复" cancel-text="取消" :confirm-loading="restoring" @ok="doRestore" width="560px">
      <a-form layout="vertical">
        <a-form-item label="备份包（zip）" required>
          <input type="file" accept=".zip" @change="e => restoreFile = e.target?.files?.[0] || null" />
        </a-form-item>
        <a-form-item label="恢复项">
          <a-checkbox-group v-model:value="restoreItems" :options="[{ label: '数据库（账号/设置/站点等）', value: 'db' }, { label: '配置（端口/入口/TLS）', value: 'config' }, { label: '托管证书', value: 'certs' }]" />
        </a-form-item>
        <WmNotice tone="warning">恢复会用上传的备份覆盖面板数据并自动重启（约数秒不可访问）；恢复前当前数据自动留存为回滚点，可随时回滚。恢复「配置」后端口/入口可能变化，请按提示的新地址访问。</WmNotice>
      </a-form>
    </a-modal>

    <!-- 中间 CA 上传 -->
    <a-modal v-model:open="caDlg" title="上传中间 CA（签发叶子证书，有效期跟随 CA）" ok-text="签发并启用 HTTPS" cancel-text="取消" @ok="applyTls('ca')">
      <a-form layout="vertical">
        <a-form-item label="CA 证书（PEM，如 mianban.crt）">
          <input type="file" accept=".crt,.pem,.cer" @change="e => readPem(e, 'cert')" />
          <a-textarea v-model:value="caForm.cert" :rows="5" placeholder="-----BEGIN CERTIFICATE-----" class="wm-mono" style="margin-top:8px" />
        </a-form-item>
        <a-form-item label="CA 私钥（PEM，如 mianban.key）">
          <input type="file" accept=".key,.pem" @change="e => readPem(e, 'key')" />
          <a-textarea v-model:value="caForm.key" :rows="4" placeholder="-----BEGIN PRIVATE KEY-----" class="wm-mono" style="margin-top:8px" />
        </a-form-item>
        <WmNotice tone="warning">CA 私钥仅保存在面板服务器本地（data/certs/ca，权限 600）。若已上传过，此处留空直接确认即可复用。</WmNotice>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import api from '../api'
import { useAuth } from '../stores/auth'

const auth = useAuth()

// ---- 设置页锚点导航（分组滚动定位 + 高亮）----
const groups = [
  { id: 'sec-account', label: '账号与安全' },
  { id: 'sec-access', label: '接入与扩展' },
  { id: 'sec-network', label: '网络与证书' },
  { id: 'sec-ops', label: '系统运维' },
]
const activeGroup = ref('sec-account')
function jumpTo(id: string) {
  activeGroup.value = id
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
function onSettingScroll() {
  const marks = groups.map(g => ({ id: g.id, top: document.getElementById(g.id)?.getBoundingClientRect().top ?? 9999 }))
  const cur = marks.filter(m => m.top <= 140).pop() || marks[0]
  if (cur) activeGroup.value = cur.id
}
onMounted(() => window.addEventListener('scroll', onSettingScroll, { passive: true }))
onUnmounted(() => window.removeEventListener('scroll', onSettingScroll))
const info = reactive<any>({})
const form = reactive<any>({ old: '', n1: '', n2: '' })
const saving = ref(false)

const sessions = ref<any[]>([])
const newUsername = ref('')
const savingName = ref(false)
const faSecret = ref('')
const faURL = ref('')
const faCode = ref('')
const faSaving = ref(false)
const upInfo = ref<any>({})
const upApplying = ref(false)

async function loadInfo() {
  Object.assign(info, (await api.get('/setting/info')) as any)
}

async function loadSessions() {
  try {
    sessions.value = (await api.get('/auth/sessions')) as any
  } catch {
    sessions.value = []
  }
}

// ---- 移动 APP 配置（docs/APP.md §7 F3）----
const appForm = reactive<any>({ public_url: '', apk_url: '', latest_version: '', release_notes: '' })
const appSaving = ref(false)

async function loadAppConfig() {
  try {
    Object.assign(appForm, (await api.get('/setting/app-config')) as any)
  } catch {
    /* 保持空值 */
  }
}

// ---- 系统软件源（apt/dnf 持久化换源）----
const sysSrc = reactive<any>({ pkg: 'apt', os_id: '', os_ver: '', current: '', backup: false, targets: [] })
const sysTarget = ref('aliyun')
const sysSrcBusy = ref(false)
const sysSrcOut = ref('')

async function loadSysSrc() {
  try {
    Object.assign(sysSrc, (await api.get('/apps/sys-source')) as any)
    if (sysTarget.value === 'aliyun' && sysSrc.current && !sysSrc.current.includes('aliyun')) {
      // 默认目标保持 aliyun，用户可自选
    }
  } catch { /* 保持空值 */ }
}

async function applySysSrc() {
  sysSrcBusy.value = true
  try {
    const d: any = await api.post('/apps/sys-source', { target: sysTarget.value }, { timeout: 200000 })
    sysSrcOut.value = d?.update || ''
    ElMessage.success('已切换到 ' + (d?.name || sysTarget.value) + ' 源')
    loadSysSrc()
  } finally {
    sysSrcBusy.value = false
  }
}

async function restoreSysSrc() {
  try {
    await ElMessageBox.confirm('将把系统软件源恢复为最初备份的原始源文件，继续？', '恢复原始源', { type: 'warning' })
  } catch { return }
  sysSrcBusy.value = true
  try {
    const d: any = await api.post('/apps/sys-source/restore', {}, { timeout: 200000 })
    sysSrcOut.value = d?.update || ''
    ElMessage.success('已恢复原始系统源')
    loadSysSrc()
  } finally {
    sysSrcBusy.value = false
  }
}

async function saveAppConfig() {
  if (auth.role !== 'admin') {
    ElMessage.warning('仅管理员可修改 APP 配置')
    return
  }
  appSaving.value = true
  try {
    await api.post('/setting/app-config', { ...appForm })
    ElMessage.success('APP 配置已保存')
  } finally {
    appSaving.value = false
  }
}

const revokingOthers = ref(false)

async function revokeOthers() {
  try {
    await ElMessageBox.confirm('将退出除当前设备外的全部登录会话（包括其它浏览器/终端/脚本），继续？', '退出其他设备', { type: 'warning' })
  } catch { return }
  revokingOthers.value = true
  try {
    const r: any = await api.post('/auth/sessions/revoke-others')
    ElMessage.success('已退出 ' + (r?.revoked ?? 0) + ' 个其他会话')
    loadSessions()
  } finally {
    revokingOthers.value = false
  }
}

async function revoke(row: any) {
  try {
    await ElMessageBox.confirm('确认踢出该会话？', '踢出', { type: 'warning' })
  } catch { return }
  await api.delete('/auth/sessions/' + row.id)
  ElMessage.success('已踢出')
  loadSessions()
}

async function saveUsername() {
  const name = newUsername.value.trim()
  if (name.length < 3) {
    ElMessage.warning('用户名至少 3 位')
    return
  }
  savingName.value = true
  try {
    await api.post('/setting/profile', { username: name })
    ElMessage.success('用户名已改为 ' + name + '，下次登录使用新用户名')
    auth.username = name
    newUsername.value = ''
  } finally {
    savingName.value = false
  }
}

async function setup2fa() {
  const d: any = await api.post('/setting/2fa/setup')
  faSecret.value = d.secret
  faURL.value = d.url
  faCode.value = ''
}

async function confirm2fa() {
  faSaving.value = true
  try {
    await api.post('/setting/2fa/enable', { secret: faSecret.value, code: faCode.value })
    ElMessage.success('两因素验证已启用，下次登录需输入动态码')
    faSecret.value = ''
  } finally {
    faSaving.value = false
  }
}

async function disable2fa() {
  try {
    await ElMessageBox.confirm('确认解除两因素验证？需要输入登录密码', '解除绑定', { type: 'warning' })
  } catch { return }
  try {
    const r: any = await ElMessageBox.prompt('输入登录密码确认解除', '密码确认', { inputType: 'password' })
    await api.post('/setting/2fa/disable', { password: r.value })
    ElMessage.success('已解除')
  } catch { /* 取消或密码错误 */ }
}

async function checkUpgrade() {
  upInfo.value = (await api.get('/setting/upgrade/check')) as any
}

async function applyUpgrade() {
  try {
    await ElMessageBox.confirm('升级会替换面板二进制并自动重启（约数秒不可访问），继续？', '在线升级', { type: 'warning' })
  } catch { return }
  upApplying.value = true
  try {
    await api.post('/setting/upgrade/apply', { binary_url: upInfo.value.binary_url, sha256: upInfo.value.sha256 || '' }, { timeout: 900000 })
    ElMessage.success('二进制已替换，面板正在重启，稍后刷新页面并核对版本')
  } finally {
    upApplying.value = false
  }
}

// ---- 面板备份导入恢复 ----
const restoreDlg = ref(false)
const restoring = ref(false)
const restoreFile = ref<File | null>(null)
const restoreItems = ref<string[]>(['db', 'config', 'certs'])
const rollbackBusy = ref(false)
const hasRollback = ref(false)

async function checkRollback() {
  try {
    const info: any = await api.get('/setting/tls/status')
    hasRollback.value = !!info.data_dir
  } catch { /* 忽略 */ }
}

async function doRestore() {
  if (!restoreFile.value) {
    ElMessage.warning('请选择备份 zip 文件')
    return
  }
  if (!restoreItems.value.length) {
    ElMessage.warning('请至少勾选一个恢复项')
    return
  }
  restoring.value = true
  try {
    const fd = new FormData()
    fd.append('file', restoreFile.value)
    restoreItems.value.forEach(i => fd.append('items', i))
    const r: any = await api.post('/setting/panel-restore', fd, { timeout: 600000, headers: { 'Content-Type': 'multipart/form-data' } })
    ElMessage.success(r?.message || '恢复任务已执行')
    restoreDlg.value = false
  } finally {
    restoring.value = false
  }
}

async function doRollback() {
  try {
    await ElMessageBox.confirm('将用最近一次恢复前的回滚点覆盖当前面板数据并重启，继续？', '回滚确认', { type: 'error' })
  } catch { return }
  rollbackBusy.value = true
  try {
    const r: any = await api.post('/setting/panel-restore/rollback', {}, { timeout: 300000 })
    ElMessage.success(r?.message || '回滚任务已执行')
  } finally {
    rollbackBusy.value = false
  }
}

function downloadBackup() {
  window.open('/api/setting/panel-backup')
}

async function submit() {
  if (!form.old || !form.n1 || !form.n2) {
    ElMessage.warning('请填写完整')
    return
  }
  if (form.n1 !== form.n2) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  if (form.n1.length < 8) {
    ElMessage.warning('新密码至少 8 位')
    return
  }
  saving.value = true
  try {
    await api.post('/setting/password', { old: form.old, new: form.n1 })
    ElMessage.success('密码已修改')
    form.old = ''
    form.n1 = ''
    form.n2 = ''
  } finally {
    saving.value = false
  }
}

// ---- 域名反代（复用 website 模块 NGINX 能力）----
const nginx = ref<any>({})
const nginxInstalling = ref(false)
const proxySite = ref<any>(null)
const certList = ref<any[]>([])
const proxyForm = reactive({ domain: '', certSel: '', forceHttps: true })
const proxySaving = ref(false)

async function loadProxy() {
  try {
    nginx.value = await api.get('/sites/nginx/status')
  } catch {
    nginx.value = { installed: false }
  }
  try {
    proxySite.value = (await api.get('/sites/panel-proxy')) as any
  } catch {
    proxySite.value = null
  }
  try {
    certList.value = (await api.get('/certs')) as any
  } catch {
    certList.value = []
  }
}

async function installNginx() {
  nginxInstalling.value = true
  try {
    await api.post('/sites/nginx/install', {}, { timeout: 700000 })
    ElMessage.success('NGINX 安装完成')
    loadProxy()
  } finally {
    nginxInstalling.value = false
  }
}

async function applyProxy() {
  const domain = proxyForm.domain.trim().toLowerCase()
  if (!domain) {
    ElMessage.warning('请填写反代域名')
    return
  }
  proxySaving.value = true
  try {
    let certPath = '', keyPath = ''
    if (proxyForm.certSel === 'panel' && tls.value.cert_path) {
      certPath = tls.value.cert_path
      keyPath = tls.value.key_path || ''
    } else if (proxyForm.certSel && proxyForm.certSel !== 'panel') {
      const c: any = certList.value.find((x: any) => x.domain === proxyForm.certSel)
      if (!c?.cert_path) {
        ElMessage.warning('所选证书缺少路径信息')
        return
      }
      certPath = c.cert_path
      keyPath = c.key_path
    }
    // 已存在则先删除旧配置再重建（幂等）
    if (proxySite.value) {
      await api.delete('/website/' + proxySite.value.site?.id)
      proxySite.value = null
    }
    await api.post('/sites', {
      name: 'panel-proxy', type: 'proxy', system: true,
      upstream: (tls.value.tls ? 'https' : 'http') + '://127.0.0.1:' + String(tls.value.listen || '').replace(':', ''),
      domains: domain,
      ssl_cert: certPath, ssl_key: keyPath,
      force_https: !!proxyForm.forceHttps && !!certPath,
      enabled: true
    }, { timeout: 120000 })
    ElMessage.success('反代已启用：请确认域名 A 记录已指向服务器公网 IP，然后用 https://' + domain + '/s/入口 访问')
    loadProxy()
  } finally {
    proxySaving.value = false
  }
}

async function toggleProxy() {
  try {
    await api.post('/sites/panel-proxy/toggle')
    ElMessage.success('已切换')
    loadProxy()
  } catch (e: any) {
    ElMessage.error(e?.message || '切换失败')
  }
}

const proxyLogDlg = ref(false)
const proxyLogName = ref('')
const proxyLogContent = ref('')

async function viewProxyLog(type: string) {
  if (!proxySite.value?.site?.id) return
  const d: any = await api.get('/sites/' + proxySite.value.site.id + '/log', { params: { type, lines: 200 } })
  proxyLogName.value = (proxySite.value.site?.domains || 'panel-proxy') + ' / ' + (type === 'error' ? '错误日志' : '访问日志')
  proxyLogContent.value = d.content || d.logs || '（暂无日志）'
  proxyLogDlg.value = true
}

async function removeProxy() {
  try {
    await ElMessageBox.confirm('移除反代站点后，域名将不再指向面板。继续？', '移除反代', { type: 'warning' })
  } catch { return }
  if (proxySite.value) await api.delete('/website/' + proxySite.value.site?.id)
  ElMessage.success('反代已移除')
  loadProxy()
}

// ---- HTTPS 证书 ----
const tls = ref<any>({})
const tlsBusy = ref('')
const caDlg = ref(false)
const extraIps = ref('')
const acmeDlg = ref(false)
const acmeForm = reactive({ domain: '', email: '' })

function openAcmeDlg() {
  acmeDlg.value = true
}

async function acmeApply() {
  if (!acmeForm.domain.trim()) {
    ElMessage.warning('请填写域名（需已解析到本服务器公网 IP）')
    return
  }
  acmeDlg.value = false
  applyTls('acme')
}
const caForm = reactive({ cert: '', key: '' })

async function loadTls() {
  try {
    tls.value = await api.get('/setting/tls/status')
  } catch {
    /* ignore */
  }
}

async function applyTls(mode: 'self' | 'ca' | 'acme') {
  if (mode === 'ca' && (!caForm.cert || !caForm.key) && !tls.value.has_ca) {
    caDlg.value = true
    return
  }
  tlsBusy.value = mode
  try {
    await api.post('/setting/tls/apply', { mode, ca_cert: caForm.cert, ca_key: caForm.key, extra_ips: extraIps.value.split(',').map((s: string) => s.trim()).filter(Boolean), extra_dns: [], acme_domain: acmeForm.domain, acme_email: acmeForm.email }, { timeout: mode === 'acme' ? 240000 : 60000 })
    ElMessage.success('证书已签发，面板正在重启为 HTTPS…约 3 秒后用 https://IP:端口/s/入口 重新打开')
  } finally {
    tlsBusy.value = ''
    caDlg.value = false
  }
}

function openCaDlg() {
  caDlg.value = true
}

function readPem(e: any, which: 'cert' | 'key') {
  const f = e.target?.files?.[0]
  if (!f) return
  const r = new FileReader()
  r.onload = () => {
    caForm[which] = String(r.result || '')
    if (which === 'cert') caForm.key = caForm.key // 保留另一侧
  }
  r.readAsText(f)
}

onMounted(async () => {
  await loadInfo()
  loadSessions()
  loadTls()
  loadProxy()
  loadAppConfig()
  loadSysSrc()
  if (auth.role === 'admin') loadApiToken()
  checkUpgrade().catch(() => {})
})

// ---- 开放 API 与 MCP ----
const panelOrigin = location.origin
const apiToken = ref<any>({ enabled: false, token: '', mode: 'full', scopes: [], readonly: false, scopes_known: [] })
const tokenRevealed = ref(false)
const tokenBusy = ref(false)
const permSaving = ref(false)

const scopeNames: Record<string, string> = {
  status: '总览仪表盘', sites: '网站', runtimes: '运行时', nodes: '节点', certs: 'SSL 证书',
  firewall: '防火墙', dbmgr: '数据库', apps: '应用商店', tasks: '计划任务', files: '文件管理',
  term: '终端', proc: '进程', notify: '通知', alerts: '告警', audit: '审计日志',
  s3: '对象存储', setting: '面板设置', auth: '账号会话', mcp: 'MCP 工具'
}
const scopeLabel = (s: string) => scopeNames[s] || s

const permForm = reactive({ mode: 'full', scopes: [] as string[], readonly: false })

const permLabel = computed(() => {
  if (!apiToken.value.enabled) return '—'
  const mode = apiToken.value.mode === 'custom'
    ? `自定义 ${apiToken.value.scopes.length} 个模块`
    : '完整权限'
  return apiToken.value.readonly ? `${mode} · 只读` : mode
})

function syncPermForm() {
  permForm.mode = apiToken.value.mode || 'full'
  permForm.scopes = [...(apiToken.value.scopes || [])]
  permForm.readonly = !!apiToken.value.readonly
}

async function loadApiToken() {
  try {
    apiToken.value = (await api.get('/mcp/token')) as any
    syncPermForm()
  } catch {
    /* 非 admin 忽略 */
  }
}

const mcpCfg = computed(() =>
  JSON.stringify(
    {
      mcpServers: {
        'hanyuxin-panel': { url: `${panelOrigin}/api/mcp`, headers: { Authorization: `Bearer ${apiToken.value.token}` } }
      }
    },
    null,
    2
  )
)

async function genApiToken() {
  if (apiToken.value.enabled) {
    try {
      await ElMessageBox.confirm('重置后已配置的 API/MCP 客户端将立即失效，需更新 Token，继续？', '重置 API Token', { type: 'warning' })
    } catch {
      return
    }
  }
  tokenBusy.value = true
  try {
    apiToken.value = (await api.post('/mcp/token', { mode: permForm.mode, scopes: permForm.scopes, readonly: permForm.readonly })) as any
    syncPermForm()
    tokenRevealed.value = true
    ElMessage.success('Token 已生成')
  } finally {
    tokenBusy.value = false
  }
}

async function savePerm() {
  if (permForm.mode === 'custom' && permForm.scopes.length === 0) {
    ElMessage.warning('请至少勾选一个模块，或改用完整权限')
    return
  }
  permSaving.value = true
  try {
    apiToken.value = (await api.put('/mcp/token', { mode: permForm.mode, scopes: permForm.scopes, readonly: permForm.readonly })) as any
    syncPermForm()
    ElMessage.success('权限已更新并立即生效')
  } finally {
    permSaving.value = false
  }
}

async function clearApiToken() {
  try {
    await ElMessageBox.confirm('禁用后所有 API/MCP 客户端立即失效，继续？', '禁用开放 API', { type: 'warning' })
  } catch {
    return
  }
  tokenBusy.value = true
  try {
    apiToken.value = (await api.delete('/mcp/token')) as any
    tokenRevealed.value = false
    ElMessage.success('已禁用')
  } finally {
    tokenBusy.value = false
  }
}
</script>

<style scoped>
/* 卡片 footer 内的说明文字靠左，按钮保持靠右 */
.st-foot-note { margin-right: auto; font-size: 12px; align-self: center; }
>
.setting-shell { display: flex; gap: 28px; align-items: flex-start; }
.setting-nav { position: sticky; top: 12px; width: 188px; flex-shrink: 0; padding: 10px 8px; background: var(--wm-surface, #fff); border: 1px solid var(--wm-border, #e5e7eb); border-radius: 12px; }
.setting-nav__title { font-weight: 700; font-size: 14px; padding: 6px 10px 10px; border-bottom: 1px solid var(--wm-border, #e5e7eb); margin-bottom: 6px; }
.setting-nav__item { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-radius: 8px; font-size: 13px; color: var(--wm-text-muted, #6b7280); cursor: pointer; transition: all .15s; }
.setting-nav__item:hover { background: var(--wm-surface-variant, #f3f4f6); color: var(--wm-text, #111827); }
.setting-nav__item--active { background: var(--wm-primary-weak, #e8f7f4); color: var(--wm-primary, #14b8a6); font-weight: 600; }
.setting-nav__dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; opacity: .5; }
.setting-nav__item--active .setting-nav__dot { opacity: 1; }
.setting-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 26px; }
.setting-group { scroll-margin-top: 16px; display: flex; flex-direction: column; gap: 14px; }
.setting-group__head { display: flex; align-items: center; gap: 8px; padding-bottom: 4px; }
.setting-group__bar { width: 4px; height: 15px; border-radius: 2px; background: var(--wm-primary, #14b8a6); }
.setting-group__head h3 { margin: 0; font-size: 15px; font-weight: 700; letter-spacing: .5px; }
@media (max-width: 1100px) { .setting-nav { display: none; } }
</style>

