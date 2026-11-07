package appstore

import "testing"

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
