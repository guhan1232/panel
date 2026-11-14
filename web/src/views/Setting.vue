<template>
  <div class="wm-page setting-page ops-page">
    <div class="wm-grid-4">
      <WmStatCard dense label="当前账号" text :value="auth.username || '未登录'" :sub="`${sessions.length} 个在线会话`" />
      <WmStatCard
        dense label="开放 API" text
        :value="apiToken.enabled ? '已启用' : '未启用'"
        :tone="apiToken.enabled ? 'success' : 'neutral'"
        :sub="apiToken.enabled ? (apiToken.readonly ? '只读权限' : '按 Token 控制') : '可在下方生成 Token 启用'"
      />
      <WmStatCard
        dense label="传输安全" text
        :value="tls.tls ? 'HTTPS' : 'HTTP'"
        :tone="tls.tls ? 'success' : 'warning'"
        :sub="tls.has_ca ? '中间 CA 已保存' : '暂无托管 CA'"
      />
      <WmStatCard
        dense label="面板版本" text
        :value="upInfo.has_update ? '可升级' : (upInfo.current || '检查中')"
        :tone="upInfo.has_update ? 'warning' : 'success'"
        :sub="upInfo.has_update ? `最新 ${upInfo.remote || '—'}` : '升级通道已连接'"
      />
    </div>

    <div class="setting-shell">
      <div class="setting-tabs">
        <a-segmented v-model:value="activeTab" :options="tabOptions" />
      </div>
      <div class="setting-main">
      <section v-if="activeTab === 'account'" class="setting-group">
        <div class="setting-group__head">
          <div><span class="setting-group__kicker">IDENTITY</span><h3>账号与安全</h3><p>管理员凭据、二次验证与设备会话。</p></div>
          <WmStatusPill label="核心配置" tone="info" :dot="false" outline />
        </div>
      <WmSectionCard class="setting-card" title="面板名称" description="自定义侧栏左上角与登录页显示的名称（1-40 字）；留空恢复默认「寒语馨面板」" :icon="SettingOutlined" :palette="{ color: 'var(--wm-p-teal)', soft: 'var(--wm-p-teal-soft)' }">
        <a-form layout="vertical" style="max-width: 440px">
          <a-form-item label="面板名称" style="margin-bottom: 0">
            <a-input v-model:value="panelNameForm" placeholder="寒语馨面板" :maxlength="40" :disabled="auth.role !== 'admin'" />
          </a-form-item>
        </a-form>
        <WmNotice v-if="auth.role !== 'admin'" tone="neutral" class="wm-mt-4">仅管理员可修改面板名称。</WmNotice>
        <template #footer>
          <a-button type="primary" :loading="savingPanelName" :disabled="auth.role !== 'admin'" @click="savePanelName">保存名称</a-button>
        </template>
      </WmSectionCard>
      <WmSectionCard class="setting-card" title="账号信息" description="修改登录用户名；初始随机用户名建议尽快改为易记名称">
        <a-form layout="vertical" style="max-width: 440px">
          <a-form-item label="当前用户名"><span class="wm-mono"><b>{{ auth.username }}</b></span></a-form-item>
          <a-form-item label="新用户名" style="margin-bottom: 0"><a-input v-model:value="newUsername" placeholder="3-64 位，字母/数字/_/-" /></a-form-item>
        </a-form>
        <WmNotice tone="neutral" class="wm-mt-4">修改后需用新用户名重新登录；SSH 首次输出的初始凭据文件将在你首次修改密码后自动删除。</WmNotice>
        <template #footer>
          <a-button type="primary" :loading="savingName" @click="saveUsername">保存用户名</a-button>
        </template>
      </WmSectionCard>
      <WmSectionCard class="setting-card" title="修改管理员密码" description="新密码至少 8 位，修改后当前会话保持有效">
        <a-form layout="vertical" style="max-width: 440px">
          <a-form-item label="原密码"><a-input-password v-model:value="form.old" /></a-form-item>
          <a-form-item label="新密码"><a-input-password v-model:value="form.n1" placeholder="至少 8 位" /></a-form-item>
          <a-form-item label="确认密码" style="margin-bottom: 0"><a-input-password v-model:value="form.n2" /></a-form-item>
        </a-form>
        <template #footer>
          <a-button type="primary" :loading="saving" @click="submit">修改密码</a-button>
        </template>
      </WmSectionCard>

      <WmSectionCard class="setting-card" title="两因素验证（TOTP）" description="启用后登录需输入验证器 App 的 6 位动态码">
        <template #actions>
          <a-button size="small" type="primary" @click="setup2fa">开始绑定</a-button>
          <a-button size="small" danger @click="disable2fa">解除绑定</a-button>
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
      <WmSectionCard class="setting-card" title="在线会话" description="当前账号在其他设备的登录会话，可踢出">
        <template #actions>
          <a-button size="small" danger :loading="revokingOthers" :disabled="!sessions.some((x: any) => !x.current)" @click="revokeOthers">退出其他设备</a-button>
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
      <section v-if="activeTab === 'access'" class="setting-group">
        <div class="setting-group__head">
          <div><span class="setting-group__kicker">INTEGRATIONS</span><h3>接入与扩展</h3><p>面向脚本、AI 客户端与移动端的连接配置。</p></div>
          <WmStatusPill :label="apiToken.enabled ? 'API 已启用' : 'API 未启用'" :tone="apiToken.enabled ? 'success' : 'neutral'" :dot="false" outline />
        </div>
      <WmSectionCard class="setting-card" title="开放 API Token" description="供外部脚本直调面板 API（Authorization: Bearer），与 MCP Token 相互独立、密钥与权限各自配置">
        <template #actions>
          <a-button size="small" @click="loadTokens">刷新</a-button>
        </template>
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="状态">
            <WmStatusPill :label="apiToken.enabled ? '已启用' : '未启用'" :tone="apiToken.enabled ? 'success' : 'warning'" :dot="false" />
          </a-descriptions-item>
          <a-descriptions-item label="Token">
            <span class="wm-mono">{{ apiToken.token ? (apiRevealed ? apiToken.token : apiToken.token.slice(0, 8) + '••••••••') : '—' }}</span>
            <a-button v-if="apiToken.token" size="small" type="link" @click="apiRevealed = !apiRevealed">{{ apiRevealed ? '隐藏' : '显示' }}</a-button>
          </a-descriptions-item>
          <a-descriptions-item label="权限">
            <WmStatusPill :label="apiPermLabel" :tone="apiToken.readonly ? 'warning' : 'neutral'" :dot="false" outline />
          </a-descriptions-item>
        </a-descriptions>
        <div class="wm-mt-4">
          <a-space wrap>
            <a-button type="primary" :loading="tokenBusy" @click="genToken('api')">{{ apiToken.enabled ? '重置 Token' : '生成 Token 启用' }}</a-button>
            <a-button v-if="apiToken.enabled" danger :loading="tokenBusy" @click="clearToken('api')">禁用开放 API</a-button>
          </a-space>
        </div>
        <template v-if="apiToken.enabled">
          <div class="wm-mt-4" style="max-width: 720px">
            <a-form layout="vertical">
              <a-form-item label="权限模式">
                <a-radio-group v-model:value="apiPermForm.mode">
                  <a-radio-button value="full">完整权限（等效管理员）</a-radio-button>
                  <a-radio-button value="custom">按模块授权</a-radio-button>
                </a-radio-group>
              </a-form-item>
              <template v-if="apiPermForm.mode === 'custom'">
                <a-form-item>
                  <a-checkbox-group v-model:value="apiPermForm.scopes" style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 6px 12px">
                    <a-checkbox v-for="s in apiToken.scopes_known || []" :key="s" :value="s">{{ scopeLabel(s) }}</a-checkbox>
                  </a-checkbox-group>
                </a-form-item>
              </template>
              <a-form-item style="margin-bottom: 0">
                <a-checkbox v-model:checked="apiPermForm.readonly">只读模式（仅允许查询类请求，所有写操作将被拒绝）</a-checkbox>
              </a-form-item>
            </a-form>
          </div>
        </template>
        <WmNotice tone="warning" class="wm-mt-4">Token 等效管理员权限：仅经 HTTPS 调用，泄露后立即重置。REST 示例：curl -H "Authorization: Bearer &lt;token&gt;" {{ panelOrigin }}/api/sites</WmNotice>
        <template v-if="apiToken.enabled" #footer>
          <span class="wm-text-muted st-foot-note">保存立即生效，Token 值不变，客户端无需重新配置</span>
          <a-button size="small" type="primary" :loading="permSaving" @click="savePerm('api')">保存权限</a-button>
        </template>
      </WmSectionCard>
      <WmSectionCard class="setting-card" title="MCP 服务" description="连接常用 AI 工具（Trae / Claude / Cursor 等），用自然语言管理面板">
        <template #actions>
          <a-button size="small" :loading="tokenBusy" @click="mcpToken.enabled ? clearToken('mcp') : genToken('mcp')">{{ mcpToken.enabled ? '停止服务' : '启用服务' }}</a-button>
        </template>
        <div class="mcp-status-row">
          <span class="mcp-status-item">MCP 服务 <WmStatusPill :label="mcpToken.enabled ? '运行正常' : '未启用'" :tone="mcpToken.enabled ? 'success' : 'neutral'" :dot="true" /></span>
          <span class="mcp-status-sep"></span>
          <span class="mcp-status-item">防火墙 <WmStatusPill :label="fwLabel" :tone="fwOk ? 'success' : 'warning'" :dot="false" /></span>
          <span class="mcp-status-sep"></span>
          <span class="mcp-status-item">HTTPS <WmStatusPill :label="tlsEnabled ? '正常' : '未启用'" :tone="tlsEnabled ? 'success' : 'warning'" :dot="false" /></span>
          <span class="mcp-status-sep"></span>
          <span class="mcp-status-item">今日调用 <b class="wm-mono">{{ mcpToken.today_calls ?? 0 }}</b> 次</span>
        </div>
        <WmSectionCard class="mcp-step" title="步骤一：设置当前 MCP 访问权限" dense quiet>
          <template #actions>
            <WmStatusPill :label="mcpPermLabel === '—' ? '未配置' : mcpPermLabel" :tone="mcpToken.readonly ? 'warning' : 'neutral'" :dot="false" outline />
          </template>
          <span class="wm-text-muted">为 MCP Token 配置权限模式与模块授权（与开放 API Token 相互独立；自定义权限需包含「MCP 工具」模块）。</span>
          <template v-if="mcpToken.enabled">
            <div class="wm-mt-4" style="max-width: 720px">
              <a-form layout="vertical">
                <a-form-item label="权限模式">
                  <a-radio-group v-model:value="mcpPermForm.mode">
                    <a-radio-button value="full">完整权限（等效管理员）</a-radio-button>
                    <a-radio-button value="custom">按模块授权</a-radio-button>
                  </a-radio-group>
                </a-form-item>
                <template v-if="mcpPermForm.mode === 'custom'">
                  <a-form-item>
                    <a-checkbox-group v-model:value="mcpPermForm.scopes" style="display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 6px 12px">
                      <a-checkbox v-for="s in mcpToken.scopes_known || []" :key="s" :value="s">{{ scopeLabel(s) }}</a-checkbox>
                    </a-checkbox-group>
                  </a-form-item>
                </template>
                <a-form-item style="margin-bottom: 0">
                  <a-checkbox v-model:checked="mcpPermForm.readonly">只读模式（MCP 工具内仅执行查询类操作）</a-checkbox>
                </a-form-item>
              </a-form>
            </div>
            <a-button size="small" type="primary" :loading="permSaving" @click="savePerm('mcp')">保存权限</a-button>
          </template>
        </WmSectionCard>
        <WmSectionCard class="mcp-step" title="步骤二：复制 MCP 安装提示词文档" dense quiet>
          <span class="wm-text-muted">获取安装提示词并粘贴至 AI 工具执行，Agent 将按文档自行完成接入（读取信息 → 处理证书 → 写入配置 → 验证）。提示词包含 Token 等敏感信息，请勿转发或泄露。</span>
          <div class="wm-mt-4">
            <a-button v-if="!promptShown" :disabled="!mcpToken.enabled" :loading="guideLoading" type="primary" ghost @click="loadAgentGuide">获取安装提示词</a-button>
            <template v-else>
              <WmCodeBlock :content="mcpPrompt" label="MCP 安装提示词（整篇复制，粘贴至 AI 工具执行）" />
              <a-space class="wm-mt-4">
                <a-button size="small" @click="copyPrompt">复制提示词</a-button>
                <a-button size="small" type="link" danger @click="promptShown = false">隐藏</a-button>
              </a-space>
            </template>
          </div>
        </WmSectionCard>
      </WmSectionCard>
      <WmSectionCard class="setting-card" title="移动 APP" description="安卓 APP 连接与自更新：对外地址用于扫码登录二维码；APK 地址供 APP 检查更新与下载">
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
          <a-button size="small" type="primary" @click="bindDlg = true">绑定移动 APP</a-button>
          <a-button type="primary" size="small" :loading="appSaving" @click="saveAppConfig">保存 APP 配置</a-button>
        </template>
      </WmSectionCard>
      </section>
      <section v-if="activeTab === 'network'" class="setting-group">
        <div class="setting-group__head">
          <div><span class="setting-group__kicker">NETWORK</span><h3>网络与证书</h3><p>加密访问、证书签发和域名反向代理。</p></div>
          <WmStatusPill :label="tls.tls ? 'HTTPS 已启用' : 'HTTP 明文'" :tone="tls.tls ? 'success' : 'warning'" :dot="false" outline />
        </div>
      <WmSectionCard class="setting-card" title="HTTPS 加密连接" description="中间 CA 签发（推荐）/ Let's Encrypt 公共免费 CA（域名）/ 纯自签 10 年兜底">
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
            <WmStatusPill :label="tls.has_ca ? '已保存（可复用续签）' : '未上传'" :tone="tls.has_ca ? 'success' : 'neutral'" :dot="false" />
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
      <WmSectionCard class="setting-card" title="域名反代访问（可选）" description="安装 NGINX 后可用自有域名反代面板并挂证书，域名 HTTPS 访问">
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
            <WmStatusPill :label="proxySite.site?.enabled ? '启用中' : '已停用'" :tone="proxySite.site?.enabled ? 'success' : 'neutral'" :dot="false" />
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
      <section v-if="activeTab === 'ops'" class="setting-group">
        <div class="setting-group__head">
          <div><span class="setting-group__kicker">MAINTENANCE</span><h3>系统运维</h3><p>软件源、版本升级与面板数据保全。</p></div>
          <WmStatusPill :label="upInfo.has_update ? '有新版本' : '维护工具'" :tone="upInfo.has_update ? 'warning' : 'neutral'" :dot="false" outline />
        </div>
      <WmSectionCard class="setting-card" title="系统软件源" description="一键把操作系统 apt/dnf 软件源换成国内镜像站并持久生效（自动备份原文件，可恢复）">
        <template #actions>
          <a-button size="small" @click="loadSysSrc">刷新</a-button>
        </template>
        <a-descriptions :column="1" bordered size="small">
          <a-descriptions-item label="系统"><span class="wm-mono">{{ sysSrc.os_id }} {{ sysSrc.os_ver }}</span></a-descriptions-item>
          <a-descriptions-item label="包管理"><span class="wm-mono">{{ sysSrc.pkg }}</span></a-descriptions-item>
          <a-descriptions-item label="当前源"><span class="wm-mono">{{ sysSrc.current || '—' }}</span></a-descriptions-item>
          <a-descriptions-item label="原始备份">
            <WmStatusPill :label="sysSrc.backup ? '已保存（可恢复）' : '未更换过'" :tone="sysSrc.backup ? 'success' : 'neutral'" :dot="false" />
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

      <WmSectionCard class="setting-card" title="面板在线升级" description="从 CNB Release 清单检查新版本：下载 → SHA256 校验 → 备份并原子替换 → 自动重启">
        <template #actions>
          <a-button size="small" :loading="upChecking" @click="checkUpgrade">检查更新</a-button>
          <a-button v-if="upInfo.has_update" size="small" type="primary" :loading="upApplying" @click="applyUpgrade">升级到 {{ upInfo.remote }}</a-button>
        </template>
        <div class="mcp-status-row">
          <span class="mcp-status-item">当前版本 <WmStatusPill :label="'v' + (upInfo.current || '—')" :tone="upInfo.has_update ? 'warning' : 'success'" :dot="true" /></span>
          <span class="mcp-status-sep"></span>
          <span class="mcp-status-item">升级渠道 <WmStatusPill label="CNB Release" tone="neutral" :dot="false" outline /></span>
          <span class="mcp-status-sep"></span>
          <span class="mcp-status-item">架构 <b class="wm-mono">{{ upInfo.arch || '-' }}</b></span>
          <span class="mcp-status-sep"></span>
          <span class="mcp-status-item">清单版本 <b class="wm-mono">{{ upInfo.remote || '—' }}</b></span>
        </div>
        <div class="wm-mt-4">
          <WmStatusPill v-if="upInfo.has_update" label="发现新版本，可升级" tone="warning" :dot="true" />
          <WmStatusPill v-else-if="upInfo.reachable" label="已是最新版本" tone="success" :dot="false" />
          <WmStatusPill v-else-if="upInfo.reachable === false" label="清单不可达" tone="warning" :dot="false" />
        </div>
        <p v-if="upInfo.notes" class="wm-text-muted wm-mt-4">{{ upInfo.notes }}</p>
        <p v-if="upInfo.err" class="wm-text-muted wm-mt-4">清单不可达：{{ upInfo.err }}（可在配置 upgrade_manifest_url 更换升级源）</p>
        <div v-if="upgrading" class="wm-mt-4">
          <WmNotice tone="warning">正在下载校验并替换二进制，面板将自动重启（约 10~30 秒不可访问），页面会在恢复后自动刷新…</WmNotice>
        </div>
        <WmNotice tone="warning" class="wm-mt-4">升级前自动备份当前二进制为 *.bak；SHA256 不符的包将被拒绝安装。</WmNotice>
      </WmSectionCard>
      <WmSectionCard class="setting-card" title="面板数据备份 / 恢复" description="导出：打包数据库快照、config.yaml 与托管证书；导入：上传备份包一键恢复（自动留存回滚点）">
        <template #actions>
          <a-button size="small" type="primary" @click="downloadBackup">导出备份包</a-button>
          <a-button size="small" @click="restoreDlg = true; restoreFile = null">导入恢复</a-button>
          <a-button v-if="hasRollback" size="small" danger :loading="rollbackBusy" @click="doRollback">回滚上次恢复前数据</a-button>
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
    <a-modal v-model:open="proxyLogDlg" :title="proxyLogName" width="860px">
      <WmCodeBlock :content="proxyLogContent" :label="proxyLogName" />
    </a-modal>

    <!-- 面板备份导入恢复 -->
    <a-modal v-model:open="restoreDlg" title="导入备份包恢复面板" ok-text="开始恢复" cancel-text="取消" :confirm-loading="restoring" @ok="doRestore" width="520px">
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
  <AppBindDialog v-model:open="bindDlg" />
</template>

<script setup lang="ts">
import {
  ApiOutlined, SettingOutlined,
  GlobalOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
  UserOutlined
} from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import AppBindDialog from '../components/AppBindDialog.vue'
import api from '../api'
import { useRoute, useRouter } from 'vue-router'
import { useAuth } from '../stores/auth'
import { useBrand } from '../stores/brand'

const auth = useAuth()
const brand = useBrand()
const route = useRoute()
const router = useRouter()

// ---- 设置页锚点导航（分组滚动定位 + 高亮）----
const tabOptions = [
  { value: 'account', label: '账号与安全' },
  { value: 'access', label: '接入与扩展' },
  { value: 'network', label: '网络与证书' },
  { value: 'ops', label: '系统运维' }
]
const activeTab = ref((route.query.tab as string) || 'account')
watch(activeTab, (t) => {
  router.replace({ query: { ...route.query, tab: t } })
})
const activeGroup = ref('sec-account')

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
const upInfo = ref<any>({ current: '', remote: '', arch: '', binary_url: '', sha256: '', notes: '', has_update: false, reachable: undefined, err: '' })
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
const bindDlg = ref(false)

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

// ---- 面板名称（自定义品牌名）----
const panelNameForm = ref(brand.name.value)
const savingPanelName = ref(false)

async function savePanelName() {
  savingPanelName.value = true
  try {
    const d: any = await api.post('/setting/name', { name: panelNameForm.value })
    brand.name.value = (d.name || '').trim() || brand.DEFAULT_PANEL_NAME
    localStorage.setItem('wm_panel_name', brand.name.value)
    document.title = brand.name.value + ' · 服务器管理面板'
    ElMessage.success('面板名称已保存')
  } finally {
    savingPanelName.value = false
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

const upgrading = ref(false)

async function applyUpgrade() {
  try {
    await ElMessageBox.confirm('升级会替换面板二进制并自动重启（约 10~30 秒不可访问），继续？', '在线升级', { type: 'warning' })
  } catch { return }
  upApplying.value = true
  try {
    await api.post('/setting/upgrade/apply', { binary_url: upInfo.value.binary_url, sha256: upInfo.value.sha256 || '' }, { timeout: 900000 })
    // 面板正在替换重启：轮询恢复后自动刷新
    upApplying.value = false
    upgrading.value = true
    ElMessage.info('面板正在重启，恢复后自动刷新页面…')
    for (let i = 0; i < 40; i++) {
      await new Promise(r => setTimeout(r, 3000))
      try {
        await api.get('/setting/brand')
        ElMessage.success('面板已升级至 v' + (upInfo.value.remote || '') + ' 并重启完成')
        setTimeout(() => location.reload(), 1200)
        return
      } catch { /* 尚未恢复，继续等 */ }
    }
    ElMessage.warning('面板重启时间较长，请稍后手动刷新页面核对版本')
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
  if (auth.role === 'admin') loadTokens()
  checkUpgrade().catch(() => {})
  try {
    await brand.load()
    panelNameForm.value = brand.name.value
  } catch { /* 保持本地缓存 */ }
})

// ---- 开放 API Token 与 MCP Token（相互独立：密钥、权限、生命周期各自配置）----
const panelOrigin = location.origin
const apiToken = ref<any>({ enabled: false, token: '', mode: 'full', scopes: [], readonly: false, scopes_known: [] })
const mcpToken = ref<any>({ enabled: false, token: '', mode: 'full', scopes: [], readonly: false, scopes_known: [] })
const apiRevealed = ref(false)
const mcpRevealed = ref(false)
const tokenBusy = ref(false)
const permSaving = ref(false)

const scopeNames: Record<string, string> = {
  status: '总览仪表盘', sites: '网站', runtimes: '运行时', nodes: '节点', certs: 'SSL 证书',
  firewall: '防火墙', dbmgr: '数据库', apps: '应用商店', tasks: '计划任务', files: '文件管理',
  term: '终端', proc: '进程', notify: '通知', alerts: '告警', audit: '审计日志',
  s3: '对象存储', setting: '面板设置', auth: '账号会话', mcp: 'MCP 工具'
}
const scopeLabel = (s: string) => scopeNames[s] || s

const apiPermForm = reactive({ mode: 'full', scopes: [] as string[], readonly: false })
const mcpPermForm = reactive({ mode: 'full', scopes: [] as string[], readonly: false })

const permLabelOf = (t: any) => {
  if (!t.enabled) return '—'
  const mode = t.mode === 'custom' ? `自定义 ${t.scopes.length} 个模块` : '完整权限'
  return t.readonly ? `${mode} · 只读` : mode
}
const apiPermLabel = computed(() => permLabelOf(apiToken.value))
const mcpPermLabel = computed(() => permLabelOf(mcpToken.value))

function syncForm(form: { mode: string; scopes: string[]; readonly: boolean }, t: any) {
  form.mode = t.mode || 'full'
  form.scopes = [...(t.scopes || [])]
  form.readonly = !!t.readonly
}

const fwStatus = ref<any>({ tool: 'none', enabled: false })
const tlsEnabled = ref(false)
const fwOk = computed(() => fwStatus.value.tool !== 'none' && fwStatus.value.enabled)
const fwLabel = computed(() => (fwStatus.value.tool === 'none' ? '未检测（不拦截）' : fwOk.value ? '已放行' : '未放行'))
const promptShown = ref(false)

const mcpPrompt = ref('')
const guideLoading = ref(false)

async function loadAgentGuide() {
  guideLoading.value = true
  try {
    const r: any = await api.get('/mcp/agent-guide')
    mcpPrompt.value = r?.markdown || ''
    promptShown.value = true
    if (!mcpPrompt.value) ElMessage.warning('提示词生成失败，请重试')
  } catch {
    ElMessage.error('获取安装提示词失败（需 admin 会话且已启用 MCP Token）')
  } finally {
    guideLoading.value = false
  }
}

async function copyPrompt() {
  try {
    await navigator.clipboard.writeText(mcpPrompt.value)
    ElMessage.success('提示词已复制，请整篇粘贴至 AI 工具执行')
  } catch {
    ElMessage.warning('复制失败，请手动选择文本复制')
  }
}

async function loadMcpEnv() {
  try { fwStatus.value = await api.get('/firewall/status') } catch { /* 忽略 */ }
  try { tlsEnabled.value = !!(await api.get('/setting/tls/status'))?.tls?.enabled } catch { /* 忽略 */ }
}

async function loadTokens() {
  try {
    apiToken.value = (await api.get('/setting/api-token')) as any
    syncForm(apiPermForm, apiToken.value)
  } catch { /* 非 admin 忽略 */ }
  try {
    mcpToken.value = (await api.get('/mcp/token')) as any
    syncForm(mcpPermForm, mcpToken.value)
    promptShown.value = false
  } catch { /* 非 admin 忽略 */ }
  loadMcpEnv()
}

const mcpCfg = computed(() =>
  JSON.stringify(
    {
      mcpServers: {
        'hanyuxin-panel': { url: `${panelOrigin}/api/mcp`, headers: { Authorization: `Bearer ${mcpToken.value.token}` } }
      }
    },
    null,
    2
  )
)

async function genToken(target: 'api' | 'mcp') {
  const st = target === 'api' ? apiToken : mcpToken
  const form = target === 'api' ? apiPermForm : mcpPermForm
  if (st.value.enabled) {
    try {
      await ElMessageBox.confirm(`重置后已配置的 ${target === 'api' ? 'API' : 'MCP'} 客户端将立即失效，需更新 Token，继续？`, '重置 Token', { type: 'warning' })
    } catch {
      return
    }
  }
  tokenBusy.value = true
  try {
    const path = target === 'api' ? '/setting/api-token' : '/mcp/token'
    const r: any = await api.post(path, { mode: form.mode, scopes: form.scopes, readonly: form.readonly })
    if (target === 'api') { apiToken.value = r; apiRevealed.value = true } else { mcpToken.value = r; promptShown.value = false }
    syncForm(form, st.value)
    ElMessage.success('Token 已生成')
  } finally {
    tokenBusy.value = false
  }
}

async function savePerm(target: 'api' | 'mcp') {
  const st = target === 'api' ? apiToken : mcpToken
  const form = target === 'api' ? apiPermForm : mcpPermForm
  if (form.mode === 'custom' && form.scopes.length === 0) {
    ElMessage.warning('请至少勾选一个模块，或改用完整权限')
    return
  }
  permSaving.value = true
  try {
    const path = target === 'api' ? '/setting/api-token' : '/mcp/token'
    const r: any = await api.put(path, { mode: form.mode, scopes: form.scopes, readonly: form.readonly })
    if (target === 'api') apiToken.value = r; else mcpToken.value = r
    syncForm(form, st.value)
    ElMessage.success('权限已更新并立即生效')
  } finally {
    permSaving.value = false
  }
}

async function clearToken(target: 'api' | 'mcp') {
  const isApi = target === 'api'
  try {
    await ElMessageBox.confirm(isApi ? '禁用后所有 API 客户端立即失效，继续？' : '禁用后所有已配置的 MCP 客户端立即失效，继续？', isApi ? '禁用开放 API' : '禁用 MCP Server', { type: 'warning' })
  } catch {
    return
  }
  tokenBusy.value = true
  try {
    const path = isApi ? '/setting/api-token' : '/mcp/token'
    const r: any = await api.delete(path)
    if (isApi) { apiToken.value = r; apiRevealed.value = false } else { mcpToken.value = r; mcpRevealed.value = false }
    ElMessage.success('已禁用')
  } finally {
    tokenBusy.value = false
  }
}
</script>

<style scoped>
.mcp-status-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 0;
  padding: 10px 14px;
  border: 1px solid var(--wm-border, #e5e7eb);
  border-radius: 10px;
  background: var(--wm-soft, rgba(13, 148, 136, 0.04));
}
.mcp-status-item { display: inline-flex; align-items: center; gap: 6px; }
.mcp-status-sep { width: 1px; height: 14px; background: var(--wm-border, #e5e7eb); margin: 0 14px; }
.mcp-step { margin-top: 12px; }
.st-foot-note { margin-right: auto; font-size: 12px; align-self: center; }

/* 布局：水平锚点条 + 全宽单列卡片流（大屏不留半卡空缺） */
.setting-tabs { margin-bottom: var(--wm-s2); }
.setting-main { min-width: 0; display: flex; flex-direction: column; gap: var(--wm-s6); }
.setting-group {
  scroll-margin-top: 64px;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--wm-s4);
}
.setting-group__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--wm-s4);
  padding: 0 2px var(--wm-s2);
  border-bottom: 1px solid var(--wm-line);
}
.setting-group__kicker {
  display: block;
  margin-bottom: 2px;
  color: var(--wm-primary);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
}
.setting-group__head h3 { margin: 0; font-size: 17px; font-weight: 650; line-height: 1.45; }
.setting-group__head p { margin-top: 2px; color: var(--wm-ink-3); font-size: 12px; line-height: 1.65; }
.setting-card :deep(.wm-section__body) { display: flex; flex-direction: column; gap: var(--wm-s3); }
.setting-card :deep(.wm-section__foot) { justify-content: flex-start; flex-wrap: wrap; }
.setting-card :deep(.ant-form-item:last-child) { margin-bottom: 0; }
.setting-card :deep(.ant-descriptions) { overflow: hidden; border-radius: var(--wm-r-md); }
.setting-card :deep(.ant-descriptions-item-label) { width: 132px; color: var(--wm-ink-3); }

@media (max-width: 980px) {
  .setting-group__head { align-items: flex-start; flex-direction: column; gap: var(--wm-s2); }
  .setting-main { gap: var(--wm-s5); }
  .setting-nav__foot { display: none; }
}
</style>
