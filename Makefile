# MicroRadar Makefile
# 构建轻量级容器监控工具

# 版本信息
VERSION := 1.0.0
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go 构建参数
GO_VERSION := 1.21.4
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)
GCFLAGS := -trimpath

# 目标架构
PLATFORMS := linux/amd64 linux/arm64
BINARY_NAME := microradar

# 目录
BUILD_DIR := build
BIN_DIR := bin
PKG_DIR := pkg

# Docker 配置
DOCKER_IMAGE := microradar
DOCKER_TAG := latest
DOCKER_REGISTRY := 

.PHONY: all build build-linux build-arm build-windows clean test lint docker docker-push help

# 默认目标
all: clean lint test build

# 本地构建 (当前平台)
build:
	@echo "构建 $(BINARY_NAME) v$(VERSION)..."
	@mkdir -p $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/microradar
	@echo "构建完成: $(BIN_DIR)/$(BINARY_NAME)"

# Linux x86_64 构建
build-linux:
	@echo "构建 Linux x86_64 版本..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/microradar
	@echo "构建完成: $(BIN_DIR)/$(BINARY_NAME)-linux-amd64"

# Linux ARM64 构建
build-arm:
	@echo "构建 Linux ARM64 版本..."
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/micro-radar
	@echo "构建完成: $(BIN_DIR)/$(BINARY_NAME)-linux-arm64"

# Windows 构建 (开发用)
build-windows:
	@echo "构建 Windows 版本..."
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -gcflags="$(GCFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/micro-radar
	@echo "构建完成: $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe"

# 多平台构建
build-all: build-linux build-arm

# 编译 eBPF 程序
build-ebpf:
	@echo "编译 eBPF 程序..."
	@mkdir -p $(BUILD_DIR)/ebpf
	clang -O2 -target bpf -c $(PKG_DIR)/ebpf/container_trace.c -o $(BUILD_DIR)/ebpf/container_trace.o
	clang -O2 -target bpf -c $(PKG_DIR)/ebpf/network_monitor.c -o $(BUILD_DIR)/ebpf/network_monitor.o
	@echo "eBPF 编译完成"

# 运行测试
test:
	@echo "运行测试..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "测试完成，覆盖率报告: coverage.html"

# 压力测试
stress-test:
	@echo "运行压力测试..."
	go test -v -run=TestStress ./test/
	@echo "压力测试完成"

# 代码检查
lint:
	@echo "运行代码检查..."
	@which golangci-lint > /dev/null || (echo "请安装 golangci-lint" && exit 1)
	golangci-lint run ./...
	@echo "代码检查完成"

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...
	@echo "代码格式化完成"

# 安全扫描
security-scan:
	@echo "运行安全扫描..."
	@which gosec > /dev/null || go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	gosec ./...
	@echo "安全扫描完成"

# Docker 镜像构建
docker:
	@echo "构建 Docker 镜像..."
	docker build -f $(BUILD_DIR)/Dockerfile.alpine -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Docker 镜像构建完成: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# 推送 Docker 镜像
docker-push: docker
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "错误: DOCKER_REGISTRY 未设置"; \
		exit 1; \
	fi
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)

# 安装依赖
deps:
	@echo "安装 Go 依赖..."
	go mod download
	go mod tidy
	@echo "依赖安装完成"

# 生成默认配置
config:
	@echo "生成默认配置文件..."
	@mkdir -p $(BIN_DIR)
	$(BIN_DIR)/$(BINARY_NAME) --init > config.yaml
	@echo "配置文件已生成: config.yaml"

# 清理构建文件
clean:
	@echo "清理构建文件..."
	rm -rf $(BIN_DIR)
	rm -rf $(BUILD_DIR)/ebpf
	rm -f coverage.out coverage.html
	@echo "清理完成"

# 安装到系统
install: build-linux
	@echo "安装到系统..."
	sudo cp $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 /usr/local/bin/$(BINARY_NAME)
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "安装完成: /usr/local/bin/$(BINARY_NAME)"

# 卸载
uninstall:
	@echo "从系统卸载..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "卸载完成"

# 检查构建环境
check-env:
	@echo "检查构建环境..."
	@go version | grep -q "$(GO_VERSION)" || (echo "警告: Go 版本不匹配，要求 $(GO_VERSION)" && go version)
	@which clang > /dev/null || (echo "错误: 需要安装 clang 编译 eBPF" && exit 1)
	@which docker > /dev/null || echo "警告: Docker 未安装，无法构建镜像"
	@echo "环境检查完成"

# 显示帮助
help:
	@echo "MicroRadar 构建系统"
	@echo ""
	@echo "可用目标:"
	@echo "  build         - 构建当前平台版本"
	@echo "  build-linux   - 构建 Linux x86_64 版本"
	@echo "  build-arm     - 构建 Linux ARM64 版本"
	@echo "  build-all     - 构建所有平台版本"
	@echo "  build-ebpf    - 编译 eBPF 程序"
	@echo "  test          - 运行测试"
	@echo "  stress-test   - 运行压力测试"
	@echo "  lint          - 代码检查"
	@echo "  fmt           - 格式化代码"
	@echo "  security-scan - 安全扫描"
	@echo "  docker        - 构建 Docker 镜像"
	@echo "  docker-push   - 推送 Docker 镜像"
	@echo "  deps          - 安装依赖"
	@echo "  config        - 生成默认配置"
	@echo "  clean         - 清理构建文件"
	@echo "  install       - 安装到系统"
	@echo "  uninstall     - 从系统卸载"
	@echo "  check-env     - 检查构建环境"
	@echo "  help          - 显示此帮助"
