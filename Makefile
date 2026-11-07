.PHONY: build build-web linux release clean vet

# 构建前端（产物输出到 web/dist，随后被 go:embed 嵌入）
build-web:
	cd web && npm install --no-audit --no-fund && npm run build

# 本机构建（嵌入当前 web/dist）
build:
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/webmblx$(shell go env GOEXE) ./cmd/server

# 面板目标平台：Linux（amd64），单二进制
linux: build-web
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/webmblx ./cmd/server

linux-arm64: build-web
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/webmblx-arm64 ./cmd/server

release: linux linux-arm64 agent agent-arm64
	cd bin && tar -czf webmblx-linux-amd64.tar.gz webmblx webmblx-agent && tar -czf webmblx-linux-arm64.tar.gz webmblx-arm64 webmblx-agent-arm64

agent: build-web
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/webmblx-agent ./cmd/agent

agent-arm64: build-web
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/webmblx-agent-arm64 ./cmd/agent

vet:
	go vet ./...

clean:
	rm -rf bin
