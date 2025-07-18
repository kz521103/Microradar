package test

import (
	"runtime"
	"testing"
	"time"

	"github.com/micro-radar/pkg/config"
	"github.com/micro-radar/pkg/ebpf"
)

// TestMemoryUsage 测试内存使用量
func TestMemoryUsage(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 100,
			MemoryLimit:   "48MB",
		},
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "test",
					Runtime:      "docker",
					SamplingRate: 2 * time.Second,
				},
			},
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	// 记录初始内存
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	initialMem := m1.Alloc

	// 启动监控
	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控失败: %v", err)
	}

	// 运行一段时间
	time.Sleep(10 * time.Second)

	// 检查内存使用
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)
	currentMem := m2.Alloc

	memUsage := currentMem - initialMem
	maxMemory := uint64(48 * 1024 * 1024) // 48MB

	t.Logf("初始内存: %d bytes", initialMem)
	t.Logf("当前内存: %d bytes", currentMem)
	t.Logf("内存增长: %d bytes (%.2f MB)", memUsage, float64(memUsage)/1024/1024)

	if currentMem > maxMemory {
		t.Errorf("内存使用超限: %d bytes > %d bytes", currentMem, maxMemory)
	}
}

// TestCPUUsage 测试 CPU 使用率
func TestCPUUsage(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 100,
		},
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "test",
					Runtime:      "docker",
					SamplingRate: 1 * time.Second,
				},
			},
		},
		Display: config.DisplayConfig{
			RefreshRate: 100 * time.Millisecond,
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控失败: %v", err)
	}

	// 监控 CPU 使用率
	start := time.Now()
	duration := 30 * time.Second

	for time.Since(start) < duration {
		time.Sleep(1 * time.Second)
		// 这里应该测量实际的 CPU 使用率
		// 目前只是确保程序能正常运行
	}

	t.Logf("CPU 测试完成，运行时间: %v", time.Since(start))
}

// TestStressContainers 压力测试：大量容器监控
func TestStressContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过压力测试")
	}

	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 500,
			MemoryLimit:   "48MB",
		},
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "stress-test",
					Runtime:      "docker",
					SamplingRate: 2 * time.Second,
				},
			},
		},
		Display: config.DisplayConfig{
			RefreshRate: 100 * time.Millisecond,
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控失败: %v", err)
	}

	// 模拟大量容器的压力测试
	start := time.Now()
	duration := 2 * time.Minute

	var maxMemory uint64
	var m runtime.MemStats

	for time.Since(start) < duration {
		runtime.ReadMemStats(&m)
		if m.Alloc > maxMemory {
			maxMemory = m.Alloc
		}

		metrics := monitor.GetMetrics()
		t.Logf("容器数量: %d, 内存使用: %.2f MB",
			len(metrics.Containers),
			float64(m.Alloc)/1024/1024)

		time.Sleep(5 * time.Second)
	}

	t.Logf("压力测试完成")
	t.Logf("最大内存使用: %.2f MB", float64(maxMemory)/1024/1024)
	t.Logf("运行时间: %v", time.Since(start))

	// 验证内存限制
	if maxMemory > 52*1024*1024 { // 52MB 峰值限制
		t.Errorf("内存使用超过峰值限制: %.2f MB > 52 MB",
			float64(maxMemory)/1024/1024)
	}
}

// TestLongRunning 长时间运行测试
func TestLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过长时间运行测试")
	}

	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 100,
			MemoryLimit:   "48MB",
		},
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "long-running-test",
					Runtime:      "docker",
					SamplingRate: 2 * time.Second,
				},
			},
		},
		Display: config.DisplayConfig{
			RefreshRate: 100 * time.Millisecond,
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控失败: %v", err)
	}

	// 运行 1 小时 (在 CI 中可能需要调整)
	duration := 1 * time.Hour
	if testing.Short() {
		duration = 5 * time.Minute
	}

	start := time.Now()
	var initialMem, maxMem, minMem uint64
	var m runtime.MemStats

	// 记录初始内存
	runtime.GC()
	runtime.ReadMemStats(&m)
	initialMem = m.Alloc
	maxMem = initialMem
	minMem = initialMem

	checkInterval := 1 * time.Minute
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runtime.ReadMemStats(&m)
			if m.Alloc > maxMem {
				maxMem = m.Alloc
			}
			if m.Alloc < minMem {
				minMem = m.Alloc
			}

			elapsed := time.Since(start)
			t.Logf("运行时间: %v, 内存: %.2f MB (初始: %.2f MB)",
				elapsed,
				float64(m.Alloc)/1024/1024,
				float64(initialMem)/1024/1024)

			if elapsed >= duration {
				goto done
			}

		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

done:
	memVariation := float64(maxMem-minMem) / float64(initialMem) * 100

	t.Logf("长时间运行测试完成")
	t.Logf("运行时间: %v", time.Since(start))
	t.Logf("初始内存: %.2f MB", float64(initialMem)/1024/1024)
	t.Logf("最大内存: %.2f MB", float64(maxMem)/1024/1024)
	t.Logf("最小内存: %.2f MB", float64(minMem)/1024/1024)
	t.Logf("内存波动: %.2f%%", memVariation)

	// 验证内存波动不超过 2.5%
	if memVariation > 2.5 {
		t.Errorf("内存波动过大: %.2f%% > 2.5%%", memVariation)
	}
}
