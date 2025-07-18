#!/bin/bash
#
# MicroRadar 安全扫描脚本
# 执行 CVE 扫描、权限检查和安全配置验证
#

set -euo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# 检查必要工具
check_tools() {
    log_info "检查安全扫描工具..."
    
    local tools=("gosec" "nancy" "trivy")
    local missing_tools=()
    
    for tool in "${tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_warn "缺少工具: ${missing_tools[*]}"
        log_info "正在安装缺少的工具..."
        
        # 安装 gosec
        if [[ " ${missing_tools[*]} " =~ " gosec " ]]; then
            go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
        fi
        
        # 安装 nancy (依赖扫描)
        if [[ " ${missing_tools[*]} " =~ " nancy " ]]; then
            go install github.com/sonatypecommunity/nancy@latest
        fi
        
        # 提示安装 trivy
        if [[ " ${missing_tools[*]} " =~ " trivy " ]]; then
            log_warn "请手动安装 trivy: https://github.com/aquasecurity/trivy"
        fi
    fi
}

# Go 代码安全扫描
scan_go_code() {
    log_info "执行 Go 代码安全扫描..."
    
    local report_file="security_report_go.json"
    
    if command -v gosec &> /dev/null; then
        gosec -fmt json -out "$report_file" ./...
        
        # 检查高危漏洞
        local high_issues=$(jq '.Issues | map(select(.severity == "HIGH")) | length' "$report_file" 2>/dev/null || echo "0")
        local medium_issues=$(jq '.Issues | map(select(.severity == "MEDIUM")) | length' "$report_file" 2>/dev/null || echo "0")
        
        log_info "发现高危问题: $high_issues"
        log_info "发现中危问题: $medium_issues"
        
        if [ "$high_issues" -gt 0 ]; then
            log_error "发现 $high_issues 个高危安全问题！"
            jq '.Issues | map(select(.severity == "HIGH"))' "$report_file" 2>/dev/null || true
            return 1
        fi
        
        log_success "Go 代码安全扫描通过"
    else
        log_warn "gosec 未安装，跳过 Go 代码扫描"
    fi
}

# 依赖漏洞扫描
scan_dependencies() {
    log_info "执行依赖漏洞扫描..."
    
    # 生成依赖列表
    go list -json -m all > go_modules.json
    
    if command -v nancy &> /dev/null; then
        nancy sleuth --loud < go_modules.json > dependency_report.txt 2>&1 || true
        
        # 检查是否有漏洞
        if grep -q "vulnerable" dependency_report.txt; then
            log_error "发现依赖漏洞！"
            cat dependency_report.txt
            return 1
        else
            log_success "依赖漏洞扫描通过"
        fi
    else
        log_warn "nancy 未安装，跳过依赖扫描"
    fi
    
    # 清理临时文件
    rm -f go_modules.json dependency_report.txt
}

# Docker 镜像安全扫描
scan_docker_image() {
    log_info "执行 Docker 镜像安全扫描..."
    
    local image_name="micro-radar:latest"
    
    # 检查镜像是否存在
    if ! docker image inspect "$image_name" &> /dev/null; then
        log_warn "Docker 镜像 $image_name 不存在，跳过镜像扫描"
        return 0
    fi
    
    if command -v trivy &> /dev/null; then
        local report_file="security_report_docker.json"
        
        trivy image --format json --output "$report_file" "$image_name"
        
        # 检查高危漏洞
        local critical_vulns=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "CRITICAL")] | length' "$report_file" 2>/dev/null || echo "0")
        local high_vulns=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "HIGH")] | length' "$report_file" 2>/dev/null || echo "0")
        
        log_info "发现严重漏洞: $critical_vulns"
        log_info "发现高危漏洞: $high_vulns"
        
        if [ "$critical_vulns" -gt 0 ] || [ "$high_vulns" -gt 0 ]; then
            log_error "Docker 镜像存在高危漏洞！"
            return 1
        fi
        
        log_success "Docker 镜像安全扫描通过"
    else
        log_warn "trivy 未安装，跳过 Docker 镜像扫描"
    fi
}

# eBPF 程序安全检查
check_ebpf_security() {
    log_info "检查 eBPF 程序安全性..."
    
    local ebpf_dir="pkg/ebpf"
    local issues=0
    
    if [ -d "$ebpf_dir" ]; then
        # 检查是否使用了不安全的函数
        local unsafe_funcs=("bpf_probe_read" "bpf_probe_write_user")
        
        for func in "${unsafe_funcs[@]}"; do
            if grep -r "$func" "$ebpf_dir" --include="*.c" &> /dev/null; then
                log_warn "发现潜在不安全的 eBPF 函数: $func"
                issues=$((issues + 1))
            fi
        done
        
        # 检查是否有适当的边界检查
        if ! grep -r "bpf_probe_read_kernel" "$ebpf_dir" --include="*.c" &> /dev/null; then
            log_warn "建议使用 bpf_probe_read_kernel 替代 bpf_probe_read"
        fi
        
        if [ $issues -eq 0 ]; then
            log_success "eBPF 程序安全检查通过"
        else
            log_warn "eBPF 程序存在 $issues 个潜在安全问题"
        fi
    else
        log_warn "eBPF 目录不存在，跳过 eBPF 安全检查"
    fi
}

# 权限检查
check_permissions() {
    log_info "检查文件权限..."
    
    local issues=0
    
    # 检查可执行文件权限
    if [ -d "bin" ]; then
        while IFS= read -r -d '' file; do
            local perms=$(stat -c "%a" "$file")
            if [ "$perms" != "755" ] && [ "$perms" != "750" ]; then
                log_warn "可执行文件权限异常: $file ($perms)"
                issues=$((issues + 1))
            fi
        done < <(find bin -type f -executable -print0 2>/dev/null)
    fi
    
    # 检查配置文件权限
    while IFS= read -r -d '' file; do
        local perms=$(stat -c "%a" "$file")
        if [ "$perms" -gt 644 ]; then
            log_warn "配置文件权限过于宽松: $file ($perms)"
            issues=$((issues + 1))
        fi
    done < <(find . -name "*.yaml" -o -name "*.yml" -o -name "*.json" -print0 2>/dev/null)
    
    if [ $issues -eq 0 ]; then
        log_success "文件权限检查通过"
    else
        log_warn "发现 $issues 个权限问题"
    fi
}

# 配置安全检查
check_config_security() {
    log_info "检查配置安全性..."
    
    local config_files=("config.yaml" "*.yml" "*.yaml")
    local issues=0
    
    for pattern in "${config_files[@]}"; do
        while IFS= read -r -d '' file; do
            # 检查是否包含敏感信息
            if grep -i "password\|secret\|key\|token" "$file" &> /dev/null; then
                log_warn "配置文件可能包含敏感信息: $file"
                issues=$((issues + 1))
            fi
            
            # 检查是否启用了调试模式
            if grep -i "debug.*true\|log_level.*debug" "$file" &> /dev/null; then
                log_warn "配置文件启用了调试模式: $file"
                issues=$((issues + 1))
            fi
        done < <(find . -name "$pattern" -type f -print0 2>/dev/null)
    done
    
    if [ $issues -eq 0 ]; then
        log_success "配置安全检查通过"
    else
        log_warn "发现 $issues 个配置安全问题"
    fi
}

# 生成安全报告
generate_report() {
    log_info "生成安全扫描报告..."
    
    local report_file="security_scan_report.md"
    
    cat > "$report_file" << EOF
# MicroRadar 安全扫描报告

生成时间: $(date)
扫描版本: $(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

## 扫描结果摘要

- Go 代码安全扫描: $([ -f security_report_go.json ] && echo "✅ 完成" || echo "⚠️ 跳过")
- 依赖漏洞扫描: $(command -v nancy &> /dev/null && echo "✅ 完成" || echo "⚠️ 跳过")
- Docker 镜像扫描: $(command -v trivy &> /dev/null && echo "✅ 完成" || echo "⚠️ 跳过")
- eBPF 程序检查: ✅ 完成
- 权限检查: ✅ 完成
- 配置安全检查: ✅ 完成

## 详细报告

详细的扫描结果请查看相应的报告文件：
- security_report_go.json (Go 代码扫描)
- security_report_docker.json (Docker 镜像扫描)

## 建议

1. 定期更新依赖库到最新版本
2. 使用最小权限原则运行程序
3. 避免在配置文件中存储敏感信息
4. 定期进行安全扫描

EOF

    log_success "安全扫描报告已生成: $report_file"
}

# 主函数
main() {
    log_info "开始 MicroRadar 安全扫描..."
    
    local exit_code=0
    
    check_tools
    
    # 执行各项安全检查
    scan_go_code || exit_code=1
    scan_dependencies || exit_code=1
    scan_docker_image || exit_code=1
    check_ebpf_security
    check_permissions
    check_config_security
    
    generate_report
    
    if [ $exit_code -eq 0 ]; then
        log_success "所有安全扫描通过！"
    else
        log_error "安全扫描发现问题，请检查报告"
    fi
    
    exit $exit_code
}

# 执行主函数
main "$@"
