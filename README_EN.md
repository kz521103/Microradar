# MicroRadar

A lightweight container monitoring tool designed for resource-constrained environments.

[中文文档](README.md) | **English**

## Core Features

- **Ultra-low Resource Usage**: Memory ≤ 48MB, CPU ≤ 4.8%
- **Fast Deployment**: From download to running in 8 minutes
- **eBPF-Powered**: Kernel-level performance monitoring with zero intrusion
- **Real-time Visualization**: Terminal interface with 15fps refresh rate
- **Multi-Runtime Support**: Docker, containerd, CRI-O

## Technical Specifications

- **Languages**: Go 1.21.4 + C (ISO C17)
- **Kernel**: eBPF (Linux 5.4+)
- **Architecture**: x86_64, ARM64
- **Container**: Alpine Linux (≤10MB image)

## Quick Start

```bash
# Download binary
curl -LO https://github.com/kz521103/Microradar/releases/tag/Source-code

# Generate configuration
./microradar init > config.yaml

# Start monitoring
./microradar --config config.yaml
```

## Monitoring Metrics

- Container CPU utilization
- Memory usage statistics
- Network latency monitoring
- TCP retransmission count

## System Requirements

- Linux 5.4+ (eBPF support)
- CAP_BPF permission or root
- Container runtime (Docker/containerd/CRI-O)

## Build

```bash
make build        # Local build
make build-arm    # ARM64 build
make docker       # Docker image
```

## Configuration

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
    network_latency: 10          # Network latency (ms)

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

## Terminal Operations

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
# Clone repository
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

## Docker Compose

```bash
# Start with monitoring stack
docker-compose --profile monitoring up -d

# Start with test containers
docker-compose --profile testing up -d

# Start everything
docker-compose --profile monitoring --profile testing up -d
```

## Troubleshooting

### Common Issues

1. **Permission Denied**
   ```bash
   # Error: Operation not permitted
   # Solution: Use root privileges or set CAP_BPF
   sudo ./micro-radar --config config.yaml
   ```

2. **eBPF Not Supported**
   ```bash
   # Check kernel version
   uname -r
   
   # Check eBPF support
   ls /sys/fs/bpf/
   ```

3. **Container Runtime Detection Failed**
   ```bash
   # Check Docker status
   systemctl status docker
   
   # Check containerd status
   systemctl status containerd
   ```

4. **High Memory Usage**
   ```yaml
   # Adjust configuration
   system:
     max_containers: 500    # Reduce monitored containers
     memory_limit: "32MB"   # Lower memory limit
   ```

### Debug Mode

```bash
# Enable debug logging
./micro-radar --config config.yaml --log-level debug

# View detailed error information
./micro-radar --config config.yaml --verbose
```

### Performance Tuning

```yaml
# Reduce sampling frequency
monitoring:
  targets:
    - sampling_rate: "5s"    # Adjust from 2s to 5s

# Lower refresh rate
display:
  refresh_rate: "200ms"      # Adjust from 100ms to 200ms
```

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   eBPF Kernel   │    │  User Space Go  │    │  Terminal UI    │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ Container   │ │◄──►│ │ Data        │ │◄──►│ │ Real-time   │ │
│ │ Tracer      │ │    │ │ Processor   │ │    │ │ Dashboard   │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ Network     │ │◄──►│ │ Metrics     │ │◄──►│ │ Alert       │ │
│ │ Monitor     │ │    │ │ Aggregator  │ │    │ │ System      │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Performance Benchmarks

| Metric | Target | Achieved |
|--------|--------|----------|
| Memory Usage | ≤ 48MB | ~42MB |
| CPU Usage | ≤ 4.8% | ~3.2% |
| Refresh Rate | ≥ 15fps | ~18fps |
| Binary Size | ≤ 7.2MB | ~6.8MB |
| Container Capacity | 1000+ | 1200+ |

## Development

### Prerequisites

- Go 1.21.4+
- Clang (for eBPF compilation)
- Linux 5.4+ (for testing)

### Build from Source

```bash
# Clone repository
git clone https://github.com/kz521103/Microradar.git
cd Microradar

# Install dependencies
make deps

# Build eBPF programs
make build-ebpf

# Build Go binary
make build

# Run tests
make test

# Security scan
make security-scan
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run security scans
6. Submit a pull request

## Roadmap

- [ ] Web dashboard interface
- [ ] Kubernetes integration
- [ ] Custom metrics plugins
- [ ] Multi-node clustering
- [ ] Historical data storage
- [ ] Machine learning anomaly detection

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Support

- **Documentation**: [https://github.com/kz521103/Microradar/wiki](https://github.com/kz521103/Microradar/wiki)
- **Issues**: [https://github.com/kz521103/Microradar/issues](https://github.com/kz521103/Microradar/issues)
- **Discussions**: [https://github.com/kz521103/Microradar/discussions](https://github.com/kz521103/Microradar/discussions)

## Acknowledgments

- [cilium/ebpf](https://github.com/cilium/ebpf) - eBPF library for Go
- [nsf/termbox-go](https://github.com/nsf/termbox-go) - Terminal interface library
- Linux kernel eBPF subsystem
