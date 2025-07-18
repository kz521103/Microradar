#!/bin/bash
#
# MicroRadar 构建脚本
# 支持多架构构建、交叉编译和优化
#

set -euo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目信息
PROJECT_NAME="microradar"
VERSION=${VERSION:-"1.0.0"}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# 构建配置
GO_VERSION="1.21.4"
BUILD_DIR="build"
BIN_DIR="bin"
DIST_DIR="dist"

# 支持的平台
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "linux/arm"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# 检查依赖
check_dependencies() {
    log_info "检查构建依赖..."
    
    # 检查 Go 版本
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装"
        exit 1
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    if [[ "$go_version" != "$GO_VERSION"* ]]; then
        log_warn "Go 版本不匹配，当前: $go_version，要求: $GO_VERSION"
    fi
    
    # 检查 clang (用于 eBPF)
    if ! command -v clang &> /dev/null; then
        log_warn "clang 未安装，无法编译 eBPF 程序"
    fi
    
    # 检查 Docker (可选)
    if ! command -v docker &> /dev/null; then
        log_warn "Docker 未安装，无法构建容器镜像"
    fi
    
    log_success "依赖检查完成"
}

# 清理构建目录
clean() {
    log_info "清理构建目录..."
    rm -rf "$BIN_DIR" "$DIST_DIR" "$BUILD_DIR/ebpf"
    mkdir -p "$BIN_DIR" "$DIST_DIR" "$BUILD_DIR/ebpf"
    log_success "清理完成"
}

# 编译 eBPF 程序
build_ebpf() {
    log_info "编译 eBPF 程序..."
    
    if ! command -v clang &> /dev/null; then
        log_warn "跳过 eBPF 编译 (clang 未安装)"
        return 0
    fi
    
    local ebpf_sources=(
        "pkg/ebpf/container_trace.c"
        "pkg/ebpf/network_monitor.c"
    )
    
    for source in "${ebpf_sources[@]}"; do
        if [[ -f "$source" ]]; then
            local output="$BUILD_DIR/ebpf/$(basename "$source" .c).o"
            log_info "编译 $source -> $output"
            
            clang -O2 -target bpf -c "$source" -o "$output" \
                -I/usr/include/linux \
                -I/usr/include \
                -Ipkg/ebpf
            
            if [[ $? -eq 0 ]]; then
                log_success "编译成功: $output"
            else
                log_error "编译失败: $source"
                exit 1
            fi
        else
            log_warn "eBPF 源文件不存在: $source"
        fi
    done
    
    log_success "eBPF 编译完成"
}

# 构建 Go 程序
build_go() {
    local goos=$1
    local goarch=$2
    local output_name=$3
    
    log_info "构建 $goos/$goarch..."
    
    # 设置构建标志
    local ldflags="-s -w -X main.Version=$VERSION -X main.Commit=$COMMIT -X main.BuildTime=$BUILD_TIME"
    local gcflags="-trimpath"
    
    # 设置环境变量
    export GOOS=$goos
    export GOARCH=$goarch
    export CGO_ENABLED=0
    
    # 构建
    go build \
        -ldflags="$ldflags" \
        -gcflags="$gcflags" \
        -o "$output_name" \
        ./cmd/microradar
    
    if [[ $? -eq 0 ]]; then
        # 获取文件大小
        local size=$(du -h "$output_name" | cut -f1)
        log_success "构建成功: $output_name ($size)"
        
        # 验证二进制文件
        if [[ "$goos" == "linux" ]]; then
            file "$output_name"
        fi
    else
        log_error "构建失败: $goos/$goarch"
        exit 1
    fi
}

# 构建单个平台
build_platform() {
    local platform=$1
    local goos=$(echo "$platform" | cut -d'/' -f1)
    local goarch=$(echo "$platform" | cut -d'/' -f2)
    
    local binary_name="$PROJECT_NAME-$goos-$goarch"
    if [[ "$goos" == "windows" ]]; then
        binary_name="$binary_name.exe"
    fi
    
    local output_path="$BIN_DIR/$binary_name"
    
    build_go "$goos" "$goarch" "$output_path"
    
    # 创建发布包
    create_release_package "$goos" "$goarch" "$output_path"
}

# 创建发布包
create_release_package() {
    local goos=$1
    local goarch=$2
    local binary_path=$3
    
    local package_name="$PROJECT_NAME-$VERSION-$goos-$goarch"
    local package_dir="$DIST_DIR/$package_name"
    
    mkdir -p "$package_dir"
    
    # 复制二进制文件
    cp "$binary_path" "$package_dir/"
    
    # 复制文档
    cp README.md "$package_dir/"
    cp QUICKSTART.md "$package_dir/"
    cp LICENSE "$package_dir/" 2>/dev/null || echo "MIT License" > "$package_dir/LICENSE"
    
    # 复制配置示例
    if [[ -f "config.yaml.example" ]]; then
        cp "config.yaml.example" "$package_dir/"
    fi
    
    # 复制 eBPF 程序 (仅 Linux)
    if [[ "$goos" == "linux" && -d "$BUILD_DIR/ebpf" ]]; then
        cp -r "$BUILD_DIR/ebpf" "$package_dir/"
    fi
    
    # 创建压缩包
    cd "$DIST_DIR"
    if [[ "$goos" == "windows" ]]; then
        zip -r "$package_name.zip" "$package_name"
        log_success "创建发布包: $package_name.zip"
    else
        tar -czf "$package_name.tar.gz" "$package_name"
        log_success "创建发布包: $package_name.tar.gz"
    fi
    cd - > /dev/null
    
    # 清理临时目录
    rm -rf "$package_dir"
}

# 构建所有平台
build_all() {
    log_info "开始多平台构建..."
    
    for platform in "${PLATFORMS[@]}"; do
        build_platform "$platform"
    done
    
    log_success "所有平台构建完成"
}

# 构建 Docker 镜像
build_docker() {
    log_info "构建 Docker 镜像..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装"
        exit 1
    fi
    
    # 确保 Linux 二进制文件存在
    local linux_binary="$BIN_DIR/$PROJECT_NAME-linux-amd64"
    if [[ ! -f "$linux_binary" ]]; then
        log_info "构建 Linux 二进制文件..."
        build_platform "linux/amd64"
    fi
    
    # 构建镜像
    docker build \
        -f build/Dockerfile.alpine \
        -t "$PROJECT_NAME:$VERSION" \
        -t "$PROJECT_NAME:latest" \
        --build-arg VERSION="$VERSION" \
        --build-arg COMMIT="$COMMIT" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        .
    
    if [[ $? -eq 0 ]]; then
        log_success "Docker 镜像构建成功"
        
        # 显示镜像信息
        docker images "$PROJECT_NAME"
    else
        log_error "Docker 镜像构建失败"
        exit 1
    fi
}

# 运行测试
run_tests() {
    log_info "运行测试..."
    
    # 单元测试
    go test -v -race -coverprofile=coverage.out ./...
    
    if [[ $? -eq 0 ]]; then
        log_success "测试通过"
        
        # 生成覆盖率报告
        go tool cover -html=coverage.out -o coverage.html
        log_info "覆盖率报告: coverage.html"
    else
        log_error "测试失败"
        exit 1
    fi
}

# 显示帮助
show_help() {
    cat << EOF
MicroRadar 构建脚本

用法: $0 [选项]

选项:
    clean           清理构建目录
    ebpf            编译 eBPF 程序
    build           构建当前平台
    build-all       构建所有平台
    docker          构建 Docker 镜像
    test            运行测试
    package         创建发布包
    help            显示此帮助

环境变量:
    VERSION         版本号 (默认: 1.0.0)
    GOOS            目标操作系统
    GOARCH          目标架构

示例:
    $0 clean build          # 清理并构建当前平台
    $0 build-all            # 构建所有平台
    $0 docker               # 构建 Docker 镜像
    VERSION=1.1.0 $0 build  # 指定版本构建

EOF
}

# 主函数
main() {
    local command=${1:-"build"}
    
    case "$command" in
        "clean")
            clean
            ;;
        "ebpf")
            build_ebpf
            ;;
        "build")
            check_dependencies
            clean
            build_ebpf
            build_platform "$(go env GOOS)/$(go env GOARCH)"
            ;;
        "build-all")
            check_dependencies
            clean
            build_ebpf
            build_all
            ;;
        "docker")
            build_docker
            ;;
        "test")
            run_tests
            ;;
        "package")
            build_all
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            log_error "未知命令: $command"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"
