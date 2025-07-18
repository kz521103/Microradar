# Contributing to MicroRadar

Thank you for your interest in contributing to MicroRadar! This document provides guidelines and information for contributors.

[中文版本](CONTRIBUTING_CN.md) | **English**

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Contributing Guidelines](#contributing-guidelines)
- [Testing](#testing)
- [Security](#security)
- [Performance Requirements](#performance-requirements)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

### Our Standards

- Use welcoming and inclusive language
- Be respectful of differing viewpoints and experiences
- Gracefully accept constructive criticism
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Prerequisites

- Go 1.21.4+
- Linux 5.4+ (for eBPF development)
- Clang/LLVM (for eBPF compilation)
- Docker (for testing)
- Git

### Development Environment

1. **Fork and Clone**
   ```bash
   git clone https://github.com/your-username/Microradar.git
   cd Microradar
   ```

2. **Install Dependencies**
   ```bash
   make deps
   ```

3. **Build eBPF Programs**
   ```bash
   make build-ebpf
   ```

4. **Build and Test**
   ```bash
   make build
   make test
   ```

## Development Setup

### Project Structure

```
Microradar/
├── cmd/microradar/          # Main application
├── pkg/
│   ├── ebpf/                 # eBPF monitoring core
│   ├── render/               # Terminal UI
│   └── config/               # Configuration management
├── build/                    # Build scripts and Dockerfiles
├── test/                     # Tests and benchmarks
├── monitoring/               # Prometheus/Grafana configs
└── docs/                     # Documentation
```

### Coding Standards

#### Go Code

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` and `golangci-lint`
- Write comprehensive tests
- Document public APIs
- Keep functions small and focused

Example:
```go
// ProcessMetrics processes container metrics and returns aggregated data.
// It returns an error if the metrics are invalid or processing fails.
func ProcessMetrics(metrics []ContainerMetric) (*AggregatedMetrics, error) {
    if len(metrics) == 0 {
        return nil, fmt.Errorf("no metrics provided")
    }
    
    // Implementation...
}
```

#### eBPF Code (C)

- Follow Linux kernel coding style
- Use proper error handling
- Add comprehensive comments
- Validate all map operations
- Use helper functions appropriately

Example:
```c
/*
 * trace_container_start - Trace container creation events
 * @ctx: tracepoint context
 *
 * This function captures container creation events and stores
 * container information in the container_map for monitoring.
 */
SEC("tracepoint/syscalls/sys_enter_clone")
int trace_container_start(struct trace_event_raw_sys_enter* ctx)
{
    // Implementation...
}
```

### Performance Requirements

All contributions must meet these performance criteria:

- **Memory Usage**: ≤ 48MB (peak ≤ 52MB)
- **CPU Usage**: ≤ 4.8% on 4-core system
- **Binary Size**: ≤ 7.2MB (stripped)
- **Refresh Rate**: ≥ 15fps for terminal UI
- **Container Capacity**: Support 1000+ containers

### Memory Management

- Use object pools for frequently allocated objects
- Implement proper cleanup in defer statements
- Monitor memory usage in tests
- Use `runtime.GC()` judiciously

Example:
```go
func (m *Monitor) processEvent() {
    event := m.memoryManager.GetFromPool("events").(*EventData)
    defer m.memoryManager.PutToPool("events", event)
    
    // Process event...
}
```

## Contributing Guidelines

### Issue Reporting

Before creating an issue:

1. Search existing issues
2. Use the issue template
3. Provide system information
4. Include reproduction steps
5. Add relevant logs

### Pull Request Process

1. **Create Feature Branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Changes**
   - Follow coding standards
   - Add tests for new functionality
   - Update documentation
   - Ensure performance requirements are met

3. **Test Your Changes**
   ```bash
   make test
   make benchmark
   make security-scan
   ```

4. **Commit Changes**
   ```bash
   git commit -m "feat: add container CPU throttling detection"
   ```

   Use conventional commit format:
   - `feat:` new features
   - `fix:` bug fixes
   - `docs:` documentation changes
   - `perf:` performance improvements
   - `test:` test additions/changes

5. **Push and Create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

### PR Requirements

- [ ] All tests pass
- [ ] Performance benchmarks meet requirements
- [ ] Security scan passes
- [ ] Documentation updated
- [ ] Changelog entry added
- [ ] Code reviewed by maintainer

### Review Process

1. Automated checks (CI/CD)
2. Code review by maintainers
3. Performance validation
4. Security review
5. Final approval and merge

## Testing

### Test Types

1. **Unit Tests**
   ```bash
   go test ./...
   ```

2. **Integration Tests**
   ```bash
   go test -tags=integration ./test/
   ```

3. **Benchmark Tests**
   ```bash
   go test -bench=. ./test/
   ```

4. **Stress Tests**
   ```bash
   go test -run=TestStress ./test/
   ```

### Test Requirements

- Minimum 80% code coverage
- All edge cases covered
- Performance tests for critical paths
- Memory leak detection
- Concurrent access testing

### Writing Tests

Example unit test:
```go
func TestContainerMetricsProcessing(t *testing.T) {
    tests := []struct {
        name     string
        input    []ContainerMetric
        expected *AggregatedMetrics
        wantErr  bool
    }{
        {
            name: "valid metrics",
            input: []ContainerMetric{
                {ID: "test1", CPUPercent: 50.0},
            },
            expected: &AggregatedMetrics{
                TotalContainers: 1,
                AvgCPU: 50.0,
            },
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := ProcessMetrics(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ProcessMetrics() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(result, tt.expected) {
                t.Errorf("ProcessMetrics() = %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## Security

### Security Guidelines

- Never commit secrets or credentials
- Validate all inputs
- Use secure coding practices
- Follow principle of least privilege
- Regular security scans

### eBPF Security

- Validate all map operations
- Check bounds on array access
- Use proper helper functions
- Avoid infinite loops
- Handle edge cases gracefully

### Reporting Security Issues

Please report security vulnerabilities to security@micro-radar.io. Do not create public issues for security problems.

## Documentation

### Documentation Requirements

- Update README.md for user-facing changes
- Add inline code comments
- Update API documentation
- Include examples for new features
- Update troubleshooting guides

### Documentation Style

- Use clear, concise language
- Include code examples
- Provide both English and Chinese versions
- Use proper markdown formatting
- Include diagrams where helpful

## Release Process

### Version Numbering

We use [Semantic Versioning](https://semver.org/):
- MAJOR: Breaking changes
- MINOR: New features (backward compatible)
- PATCH: Bug fixes (backward compatible)

### Release Checklist

- [ ] All tests pass
- [ ] Performance benchmarks meet requirements
- [ ] Security scan clean
- [ ] Documentation updated
- [ ] Changelog updated
- [ ] Version bumped
- [ ] Release notes prepared

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Email**: security@micro-radar.io (security issues only)

### Getting Help

1. Check existing documentation
2. Search GitHub issues
3. Create a new issue with details
4. Join community discussions

## Recognition

Contributors will be recognized in:
- CONTRIBUTORS.md file
- Release notes
- Project documentation

Thank you for contributing to MicroRadar!
