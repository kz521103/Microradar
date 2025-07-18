# MicroRadar 快速开始指南

## 系统要求

- **操作系统**: Linux 5.4+ (支持 eBPF)
- **架构**: x86_64 或 ARM64
- **权限**: CAP_BPF 或 root 权限
- **容器运行时**: Docker、containerd 或 CRI-O

## 部署步骤

### 1. 下载二进制文件

```bash
# 下载最新版本 (Linux x86_64)
curl -LO https://github.com/kz521103/Microradar/releases/download/Source-code/microradar-linux-amd64

# 或下载 ARM64 版本
curl -LO https://github.com/kz521103/Microradar/releases/download/Source-code/microradar-linux-arm64

# 重命名并设置权限
mv microradar-linux-amd64 microradar
chmod +x microradar
```

### 2. 生成配置文件

```bash
# 生成默认配置
./micro-radar --init > config.yaml

# 查看配置内容
cat config.yaml
```

### 3. 启动监控

```bash
# 交互模式 (推荐用于开发和调试)
./micro-radar --config config.yaml

# 守护进程模式 (推荐用于生产环境)
./micro-radar --config config.yaml --daemon
```

## 配置说明

### 基础配置

```yaml
monitoring:
  targets:
    - name: "production-cluster"
      runtime: "docker"          # 容器运行时: docker, containerd, cri-o
      metrics:                   # 监控指标
        - cpu
        - memory
        - network_latency
        - tcp_retransmits
      sampling_rate: "2s"        # 采样间隔: 1s, 2s, 5s

  alert_thresholds:              # 告警阈值
    cpu: 70.0                    # CPU 使用率 (%)
    memory: 80.0                 # 内存使用率 (%)
    network_latency: 10          # 网络延迟 (毫秒)

display:
  refresh_rate: "100ms"          # 终端刷新间隔
  theme: "default"               # 主题: default, dark, light

system:
  max_containers: 1000           # 最大监控容器数
  memory_limit: "48MB"           # 内存使用限制
  log_level: "info"              # 日志级别: debug, info, warn, error
```

### 高级配置

```yaml
# 多运行时监控
monitoring:
  targets:
    - name: "docker-containers"
      runtime: "docker"
      sampling_rate: "2s"
    
    - name: "k8s-pods"
      runtime: "containerd"
      sampling_rate: "1s"
      metrics:
        - cpu
        - memory
        - network_latency

# 自定义告警阈值
  alert_thresholds:
    cpu: 80.0              # 更严格的 CPU 阈值
    memory: 90.0           # 更宽松的内存阈值
    network_latency: 5     # 更严格的延迟阈值
```

## 终端操作指南

### 快捷键

| 按键 | 功能 |
|------|------|
| `1` | 切换到容器视图 |
| `2` | 切换到网络视图 |
| `3` | 切换到系统视图 |
| `↑/↓` | 选择容器 |
| `K` 或 `Del` | 取消选中的容器进程 |
| `Enter` | 确认操作 |
| `F1` | 显示帮助 |
| `F2` | 切换视图 |
| `F5` | 强制刷新 |
| `Ctrl+L` | 清除警告 |
| `Q` 或 `Esc` | 退出程序 |

### 界面说明

```text
[MicroRadar] - PID: 142857 | Uptime: 12:34:56 | Containers: 15
┌─────────────┬───────┬───────┬───────────┬──────────┐
│ CONTAINER   │ CPU%  │ MEM%  │ NET_LAT   │ STATUS   │
├─────────────┼───────┼───────┼───────────┼──────────┤
│ web-server  │ 32.1  │ 45.6  │ 8ms       │ running  │
│ db-primary  │ 78.9  │ 62.3  │ 12ms ⚠️   │ running  │
│ cache-redis │ 15.2  │ 25.8  │ 3ms       │ running  │
└─────────────┴───────┴───────┴───────────┴──────────┘
[1] Containers [2] Network [3] System [Q] Quit
```

### 告警标识

- **⚠️** : CPU ≥ 70% 或网络延迟 ≥ 10ms
- **🔴** : 内存使用率 ≥ 80%
- **⚡** : 网络异常或高延迟

## Docker 部署

### 构建镜像

```bash
# 克隆代码
git clone https://github.com/kz521103/Microradar.git
cd Microradar

# 构建 Docker 镜像
make docker
```

### 运行容器

```bash
# 创建配置文件
mkdir -p /opt/microradar/config
./micro-radar --init > /opt/microradar/config/config.yaml

# 运行容器 (需要特权模式访问 eBPF)
docker run -d \
  --name microradar \
  --privileged \
  --pid host \
  --network host \
  -v /opt/microradar/config:/app/config \
  -v /var/run/docker.sock:/var/run/docker.sock \
  micro-radar:latest
```

### 健康检查

```bash
# 检查容器状态
docker ps | grep microradar

# 查看日志
docker logs microradar

# 访问健康检查端点
curl http://localhost:8080/health
```

## 故障排除

### 常见问题

1. **权限不足**
   ```bash
   # 错误: Operation not permitted
   # 解决: 使用 root 权限或设置 CAP_BPF
   sudo ./micro-radar --config config.yaml
   ```

2. **eBPF 不支持**
   ```bash
   # 检查内核版本
   uname -r
   
   # 检查 eBPF 支持
   ls /sys/fs/bpf/
   ```

3. **容器运行时检测失败**
   ```bash
   # 检查 Docker 状态
   systemctl status docker
   
   # 检查 containerd 状态
   systemctl status containerd
   ```

4. **内存使用过高**
   ```yaml
   # 调整配置文件
   system:
     max_containers: 500    # 减少监控容器数
     memory_limit: "32MB"   # 降低内存限制
   ```

### 调试模式

```bash
# 启用调试日志
./micro-radar --config config.yaml --log-level debug

# 查看详细错误信息
./micro-radar --config config.yaml --verbose
```

### 性能调优

```yaml
# 降低采样频率
monitoring:
  targets:
    - sampling_rate: "5s"    # 从 2s 调整到 5s

# 降低刷新率
display:
  refresh_rate: "200ms"      # 从 100ms 调整到 200ms
```

## 卸载

```bash
# 停止服务
sudo systemctl stop microradar

# 删除二进制文件
sudo rm /usr/local/bin/micro-radar

# 删除配置文件
sudo rm -rf /etc/microradar/

# 删除 Docker 镜像
docker rmi micro-radar:latest
```

- **问题反馈**: [https://github.com/kz521103/Microradar/issues](https://github.com/kz521103/Microradar/issues)

## 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件
