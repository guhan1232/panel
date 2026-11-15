package rtm

import (
	"strings"
	"testing"
)

func TestParseNodeIndex(t *testing.T) {
	data := []byte(`[
		{"version":"v18.20.4","date":"2024-05-01","lts":"Hydrogen"},
		{"version":"v22.0.0-rc.1","date":"2024-04-20","lts":false},
		{"version":"v22.3.0","date":"2024-06-20","lts":false}
	]`)
	out := parseNodeIndex(data, "https://mirrors.aliyun.com/nodejs-release", "x64")
	if len(out) != 2 {
		t.Fatalf("应过滤 rc 版本, got %d", len(out))
	}
	if out[0].Version != "v22.3.0" || !out[1].LTS {
		t.Errorf("排序/LTS 解析错误: %+v", out)
	}
	if out[0].URL != "https://mirrors.aliyun.com/nodejs-release/v22.3.0/node-v22.3.0-linux-x64.tar.gz" {
		t.Errorf("URL 错误: %s", out[0].URL)
	}
}

func TestParseGoListing(t *testing.T) {
	html := `<a href="/golang/go1.23.4.linux-amd64.tar.gz">go1.23.4.linux-amd64.tar.gz</a>
<a href="/golang/go1.22.10.linux-amd64.tar.gz">go1.22.10.linux-amd64.tar.gz</a>
<a href="/golang/go1.23rc2.linux-amd64.tar.gz">go1.23rc2</a>`
	out := parseGoListing(html, "https://mirrors.aliyun.com/golang", "amd64")
	if len(out) != 2 || out[0].Version != "1.23.4" {
		t.Fatalf("目录解析/排序错误: %+v", out)
	}
	if out[0].URL != "https://mirrors.aliyun.com/golang/go1.23.4.linux-amd64.tar.gz" {
		t.Errorf("URL 错误: %s", out[0].URL)
	}
}

func TestJdkVerFromFile(t *testing.T) {
	v, ok := jdkVerFromFile("OpenJDK17U-jdk_x64_linux_hotspot_17.0.12_7.tar.gz")
	if !ok || v != "17.0.12+7" {
		t.Errorf("jdkVerFromFile = %q %v", v, ok)
	}
	if _, ok := jdkVerFromFile("not-a-runtime.tar.gz"); ok {
		t.Errorf("非法文件名不应解析成功")
	}
}

func TestBinsFor(t *testing.T) {
	n := binsFor("nodejs", "v20.18.1", "x64")
	if n[0].Rel != "node-v20.18.1-linux-x64/bin/node" {
		t.Errorf("node bins 错误: %+v", n)
	}
	j := binsFor("openjdk", "17.0.12+7", "x64")
	if j[0].Rel != "jdk-17.0.12+7/bin/java" {
		t.Errorf("openjdk bins 错误: %+v", j)
	}
}

func TestParseToolchainList(t *testing.T) {
	list := `v0.0.1-go1.2.2.linux-386
v0.0.1-go1.23.4.linux-amd64
v0.0.1-go1.27.3.linux-amd64
v0.0.1-go1.27.3.linux-arm64
v0.0.1-go1.27.3.darwin-amd64
v0.0.1-go1.24rc1.linux-amd64`
	out := parseToolchainList(list, "amd64")
	if len(out) != 2 {
		t.Fatalf("应保留 2 个(过滤他平台+rc), got %d", len(out))
	}
	if out[0].Version != "1.27.3" || !strings.HasSuffix(out[0].URL, "/v0.0.1-go1.27.3.linux-amd64.zip") {
		t.Errorf("解析错误: %+v", out[0])
	}
}
