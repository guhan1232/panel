package appstore

import (
	"strings"
	"testing"
)

func TestProxyImage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"mysql:8", "docker.m.daocloud.io/library/mysql:8"},
		{"nginx", "docker.m.daocloud.io/library/nginx"},
		{"user/app:1.0", "docker.m.daocloud.io/user/app:1.0"},
		{"docker.io/library/redis:7", "docker.m.daocloud.io/library/redis:7"},
		{"ghcr.io/x/y:2", "docker.m.daocloud.io/ghcr.io/x/y:2"},
		{"localhost:5000/a", "docker.m.daocloud.io/localhost:5000/a"},
		{"registry.example.com:443/a/b", "docker.m.daocloud.io/registry.example.com:443/a/b"},
	}
	for _, c := range cases {
		if got := proxyImage(c.in, "docker.m.daocloud.io"); got != c.want {
			t.Errorf("proxyImage(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestApplyDockerMirror(t *testing.T) {
	yaml := `services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: "p"
  cache:
    image: redis:7-alpine
`
	want := `services:
  db:
    image: docker.m.daocloud.io/library/mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: "p"
  cache:
    image: docker.m.daocloud.io/library/redis:7-alpine
`
	if got := applyDockerMirror(yaml, "docker.m.daocloud.io"); got != want {
		t.Errorf("applyDockerMirror mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestApplyGitHubMirror(t *testing.T) {
	u := "https://github.com/caddyserver/caddy/releases/download/v2.8.4/caddy_2.8.4_linux_amd64.tar.gz"
	want := "https://ghfast.top/" + u
	if got := applyGitHubMirror(u, "https://ghfast.top/"); got != want {
		t.Errorf("applyGitHubMirror = %q, want %q", got, want)
	}
	if got := applyGitHubMirror("https://dl.min.io/server/minio/release/linux-amd64/minio", "https://ghfast.top"); got != "https://dl.min.io/server/minio/release/linux-amd64/minio" {
		t.Errorf("非 GitHub 地址不应被改写: %q", got)
	}
}

func TestSubstitute(t *testing.T) {
	s := substitute("image: {{image}}\nport: {{port}}", map[string]string{"image": "a:1", "port": "8080"})
	if s != "image: a:1\nport: 8080" {
		t.Errorf("substitute = %q", s)
	}
}

func TestResolveMirror(t *testing.T) {
	if m, note := resolveMirror("", "compose"); m.Docker != "docker.m.daocloud.io" || note == "" {
		t.Errorf("auto compose 应选中 DaoCloud, got %+v %q", m, note)
	}
	if m, _ := resolveMirror("", "native"); m.GitHub == "" {
		t.Errorf("auto native 应选中 GitHub 加速源")
	}
	if m, _ := resolveMirror("off", "compose"); m.Docker != "" || m.Name != "官方直连" {
		t.Errorf("off 应返回官方直连, got %+v", m)
	}
	if m, _ := resolveMirror("nonexistent", "native"); m.Name != "官方直连" {
		t.Errorf("未知 ID 应回退官方直连, got %+v", m)
	}
}

func TestResolveNativeURL(t *testing.T) {
	urls := map[string]string{
		"amd64": "https://github.com/x/y/releases/download/v1/y-v1-linux-amd64.tar.gz",
	}
	mirrorPaths := map[string]map[string]string{
		"aliyun": {"amd64": "nodejs-release/v20.18.1/node-v20.18.1-linux-x64.tar.gz"},
	}
	p := map[string]string{}

	// auto + GitHub → 加速前缀
	u, note, err := resolveNativeURL(urls, mirrorPaths, "amd64", "auto", p)
	if err != nil || !strings.HasPrefix(u, "https://ghfast.top/https://github.com/") || note == "" {
		t.Errorf("auto github: u=%q note=%q err=%v", u, note, err)
	}
	// official
	u, _, err = resolveNativeURL(urls, mirrorPaths, "amd64", "official", p)
	if err != nil || !strings.HasPrefix(u, "https://github.com/") {
		t.Errorf("official: u=%q err=%v", u, err)
	}
	// ghfast 指定
	u, _, err = resolveNativeURL(urls, mirrorPaths, "amd64", "ghfast", p)
	if err != nil || !strings.HasPrefix(u, "https://ghfast.top/") {
		t.Errorf("ghfast: u=%q err=%v", u, err)
	}
	// 镜像站命中（注意 node 的 x64 命名由 MirrorPaths 值本身承载）
	nodeURLs := map[string]string{"amd64": "https://nodejs.org/dist/v20.18.1/node-v20.18.1-linux-x64.tar.gz"}
	u, note, err = resolveNativeURL(nodeURLs, mirrorPaths, "amd64", "aliyun", p)
	if err != nil || u != "https://mirrors.aliyun.com/nodejs-release/v20.18.1/node-v20.18.1-linux-x64.tar.gz" {
		t.Errorf("aliyun: u=%q err=%v", u, err)
	}
	_ = note
	// 未收录镜像站
	if _, _, err = resolveNativeURL(urls, mirrorPaths, "amd64", "tuna", p); err == nil {
		t.Errorf("tuna 未收录应报错")
	}
	// 镜像站无该架构
	if _, _, err = resolveNativeURL(nodeURLs, map[string]map[string]string{"aliyun": {"arm64": "x"}}, "amd64", "aliyun", p); err == nil {
		t.Errorf("无架构应报错")
	}
	// 非 GitHub 应用选 GitHub 加速 → 报错
	if _, _, err = resolveNativeURL(map[string]string{"amd64": "https://dl.min.io/a"}, nil, "amd64", "ghfast", p); err == nil {
		t.Errorf("非 GitHub 选加速应报错")
	}
	// 参数替换
	if u, _, _ = resolveNativeURL(map[string]string{"amd64": "https://mirrors.aliyun.com/golang/go{{ver}}.tar.gz"},
		map[string]map[string]string{"aliyun": {"amd64": "golang/go{{ver}}.tar.gz"}}, "amd64", "aliyun", map[string]string{"ver": "1.23.4"}); u != "https://mirrors.aliyun.com/golang/go1.23.4.tar.gz" {
		t.Errorf("参数替换失败: %q", u)
	}
}
