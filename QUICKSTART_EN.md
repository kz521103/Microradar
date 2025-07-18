# MicroRadar Quick Start Guide

[中文版本](QUICKSTART.md) | **English**

## System Requirements

- **Operating System**: Linux 5.4+ (eBPF support)
- **Architecture**: x86_64 or ARM64
- **Permissions**: CAP_BPF or root privileges
- **Container Runtime**: Docker, containerd, or CRI-O

## Deployment Steps

### 1. Download Binary

```bash
# Download latest version (Linux x86_64)
curl -LO https://github.com/kz521103/Microradar/releases/download/Source-code/microradar-linux-amd64

# Or download ARM64 version
curl -LO https://github.com/kz521103/Microradar/releases/download/Source-code/microradar-linux-arm64

# Rename and set permissions
mv microradar-linux-amd64 microradar
chmod +x micro-radar
```

### 2. Generate Configuration

```bash
# Generate default configuration
./micro-radar --init > config.yaml

# View configuration content
cat config.yaml
```

### 3. Start Monitoring

```bash
# Interactive mode (recommended for development and debugging)
./micro-radar --config config.yaml

# Daemon mode (recommended for production)
./micro-radar --config config.yaml --daemon
```

## Configuration Guide

### Basic Configuration

```yaml
monitoring:
  targets:
    - name: "production-cluster"
      runtime: "docker"          # Container runtime: docker, containerd, cri-o
      metrics:                   # Monitoring metrics
        - cpu
        - memory
        - network_latency
        - tcp_retransmits
      sampling_rate: "2s"        # Sampling interval: 1s, 2s, 5s

  alert_thresholds:              # Alert thresholds
    cpu: 70.0                    # CPU usage (%)
    memory: 80.0                 # Memory usage (%)
    network_latency: 10          # Network latency (milliseconds)

display:
  refresh_rate: "100ms"          # Terminal refresh interval
  theme: "default"               # Theme: default, dark, light

system:
  max_containers: 1000           # Maximum containers to monitor
  memory_limit: "48MB"           # Memory usage limit
  log_level: "info"              # Log level: debug, info, warn, error
```

### Advanced Configuration

```yaml
# Multi-runtime monitoring
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

# Custom alert thresholds
  alert_thresholds:
    cpu: 80.0              # Stricter CPU threshold
    memory: 90.0           # Looser memory threshold
    network_latency: 5     # Stricter latency threshold
```

## Terminal Operations Guide

### Keyboard Shortcuts

| Key | Function |
|-----|----------|
| `1` | Switch to container view |
| `2` | Switch to network view |
| `3` | Switch to system view |
| `↑/↓` | Select container |
| `K` or `Del` | Kill selected container process |
| `Enter` | Confirm action |
| `F1` | Show help |
| `F2` | Switch view |
| `F5` | Force refresh |
| `Ctrl+L` | Clear warnings |
| `Q` or `Esc` | Exit program |

### Interface Description

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

### Alert Indicators

- **⚠️** : CPU ≥ 70% or network latency ≥ 10ms
- **🔴** : Memory usage ≥ 80%
- **⚡** : Network anomaly or high latency

## Docker Deployment

### Build Image

```bash
# Clone code
git clone https://github.com/kz521103/Microradar.git
cd Microradar

# Build Docker image
make docker
```

### Run Container

```bash
# Create configuration file
mkdir -p /opt/microradar/config
./micro-radar --init > /opt/microradar/config/config.yaml

# Run container (requires privileged mode for eBPF access)
docker run -d \
  --name microradar \
  --privileged \
  --pid host \
  --network host \
  -v /opt/microradar/config:/app/config \
  -v /var/run/docker.sock:/var/run/docker.sock \
  micro-radar:latest
```

### Health Check

```bash
# Check container status
docker ps | grep microradar

# View logs
docker logs microradar

# Access health check endpoint
curl http://localhost:8080/health
```

## Docker Compose Deployment

### Basic Setup

```bash
# Start MicroRadar only
docker-compose up -d microradar

# Start with monitoring stack (Prometheus + Grafana)
docker-compose --profile monitoring up -d

# Start with test containers
docker-compose --profile testing up -d
```

### Full Stack

```bash
# Start everything
docker-compose --profile monitoring --profile testing up -d

# View services
docker-compose ps

# View logs
docker-compose logs -f microradar
```

### Access Services

- **MicroRadar API**: http://localhost:8080
- **Prometheus**: http://localhost:9091
- **Grafana**: http://localhost:3000 (admin/admin)

## Troubleshooting

### Common Issues

1. **Permission Denied**
   ```bash
   # Error: Operation not permitted
   # Solution: Use root privileges or set CAP_BPF
   sudo ./micro-radar --config config.yaml
   
   # Or set capabilities
   sudo setcap cap_bpf+ep ./micro-radar
   ```

2. **eBPF Not Supported**
   ```bash
   # Check kernel version
   uname -r
   
   # Check eBPF support
   ls /sys/fs/bpf/
   
   # Check if CONFIG_BPF is enabled
   grep CONFIG_BPF /boot/config-$(uname -r)
   ```

3. **Container Runtime Detection Failed**
   ```bash
   # Check Docker status
   systemctl status docker
   
   # Check containerd status
   systemctl status containerd
   
   # Check CRI-O status
   systemctl status crio
   
   # Verify socket permissions
   ls -la /var/run/docker.sock
   ```

4. **High Memory Usage**
   ```yaml
   # Adjust configuration file
   system:
     max_containers: 500    # Reduce monitored containers
     memory_limit: "32MB"   # Lower memory limit
   
   monitoring:
     targets:
       - sampling_rate: "5s"  # Reduce sampling frequency
   ```

5. **Network Issues**
   ```bash
   # Check firewall settings
   sudo iptables -L
   
   # Check if ports are available
   netstat -tlnp | grep :8080
   
   # Test connectivity
   curl -v http://localhost:8080/health
   ```

### Debug Mode

```bash
# Enable debug logging
./micro-radar --config config.yaml --log-level debug

# View detailed error information
./micro-radar --config config.yaml --verbose

# Check eBPF program loading
dmesg | grep bpf
```

### Performance Tuning

```yaml
# Reduce resource usage
monitoring:
  targets:
    - sampling_rate: "5s"    # From 2s to 5s

display:
  refresh_rate: "200ms"      # From 100ms to 200ms

system:
  max_containers: 500        # Reduce from 1000
  memory_limit: "32MB"       # Reduce from 48MB
```

### Log Analysis

```bash
# View system logs
journalctl -u microradar -f

# View container logs
docker logs -f microradar

# Check eBPF logs
dmesg | tail -n 50 | grep -i bpf
```

## Advanced Usage

### Custom Metrics

```yaml
# Enable additional metrics
monitoring:
  targets:
    - metrics:
        - cpu
        - memory
        - network_latency
        - tcp_retransmits
        - disk_io          # Additional metric
        - network_io       # Additional metric
```

### Multi-Cluster Monitoring

```yaml
# Monitor multiple clusters
monitoring:
  targets:
    - name: "prod-cluster"
      runtime: "docker"
      sampling_rate: "2s"
    
    - name: "staging-cluster"
      runtime: "containerd"
      sampling_rate: "5s"
    
    - name: "dev-cluster"
      runtime: "cri-o"
      sampling_rate: "10s"
```

### Integration with Monitoring Stack

```bash
# Export metrics to Prometheus
curl http://localhost:8080/metrics

# View in Grafana
# Import dashboard from monitoring/grafana/dashboards/
```

## API Reference

### Health Check

```bash
GET /health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Metrics Endpoint

```bash
GET /metrics
```

Response (Prometheus format):
```
# HELP microradar_containers_total Total number of monitored containers
# TYPE microradar_containers_total gauge
microradar_containers_total 15

# HELP microradar_container_cpu_percent Container CPU usage percentage
# TYPE microradar_container_cpu_percent gauge
microradar_container_cpu_percent{container_id="abc123",container_name="web-server"} 32.1
```

### Status Endpoint

```bash
GET /status
```

Response:
```json
{
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "containers_monitored": 15,
  "memory_usage_mb": 42.5,
  "ebpf_maps_count": 5,
  "last_update": "2024-01-15T10:30:00Z"
}
```

## Uninstall

```bash
# Stop service
sudo systemctl stop microradar

# Remove binary
sudo rm /usr/local/bin/micro-radar

# Remove configuration
sudo rm -rf /etc/microradar/

# Remove Docker image
docker rmi micro-radar:latest

# Remove Docker Compose stack
docker-compose down -v
```

- **Issue Feedback**:[https://github.com/kz521103/Microradar/issues](https://github.com/kz521103/Microradar/issues)


## License

MIT License - see [LICENSE](LICENSE) file for details.
