.PHONY: build run test lint clean wire vet

# ── 构建 ──
build:
	go build -o bin/sion ./cmd/sion

# ── 运行 ──
run:
	go run ./cmd/sion settings

run-pet:
	go run ./cmd/sion pet

# ── 测试 ──
test:
	go test ./internal/... -v -count=1

test-domain:
	go test ./internal/domain/... -v

test-adapter:
	go test ./internal/adapter/... -v

# ── 代码检查 ──
vet:
	go vet ./...

lint:
	golangci-lint run ./...

# ── Wire 依赖注入代码生成 ──
wire:
	cd internal/app && wire

# ── 清理 ──
clean:
	rm -rf bin/

# ── 开发 ──
dev:
	go run ./cmd/sion settings

# ── 工具安装 ──
tools:
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
