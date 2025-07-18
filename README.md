# MicroRadar (微观雷达)

轻量级容器监控工具，专为资源受限环境设计。

## 核心特性

- **极低资源占用**: 内存 ≤ 48MB，CPU ≤ 4.8%
- **快速部署**: 8分钟内从下载到运行
- **eBPF驱动**: 内核级性能监控，零侵入
- **实时可视化**: 终端界面，15fps刷新率
- **多运行时支持**: Docker、containerd、CRI-O

## 技术规格

- **语言**: Go 1.21.4 + C (ISO C17)
- **内核**: eBPF (Linux 5.4+)
- **架构**: x86_64, ARM64
- **容器**: Alpine Linux (≤10MB镜像)

## 快速开始

```bash
# 下载二进制
curl -LO https://github.com/kz521103/Microradar/releases/tag/Source-code

# 生成配置
./microradar init > config.yaml

# 启动监控
./microradar --config config.yaml
```

## 监控指标

- 容器CPU使用率
- 内存占用统计
- 网络延迟监控
- TCP重传计数

## 系统要求

- Linux 5.4+ (eBPF支持)
- CAP_BPF权限或root
- 容器运行时 (Docker/containerd/CRI-O)

## 构建

```bash
make build        # 本地构建
make build-arm    # ARM64构建
make docker       # Docker镜像
```

## 许可证

MIT License
