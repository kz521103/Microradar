package test

import (
	"fmt"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/kz521103/Microradar/pkg/config"
	"github.com/kz521103/Microradar/pkg/ebpf"
	"github.com/kz521103/Microradar/pkg/render"
)

// BenchmarkMonitorCreation 基准测试：监控器创建
func BenchmarkMonitorCreation(b *testing.B) {
	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 100,
			MemoryLimit:   "48MB",
		},
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "benchmark",
					Runtime:      "docker",
					SamplingRate: 2 * time.Second,
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		monitor, err := ebpf.NewMonitor(cfg)
		if err != nil {
			b.Fatalf("创建监控器失败: %v", err)
		}
		monitor.Close()
	}
}

// BenchmarkMetricsUpdate 基准测试：指标更新
func BenchmarkMetricsUpdate(b *testing.B) {
	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 1000,
		},
		Display: config.DisplayConfig{
			RefreshRate: 100 * time.Millisecond,
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		b.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		b.Fatalf("启动监控器失败: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics := monitor.GetMetrics()
		if metrics == nil {
			b.Fatal("获取指标失败")
		}
	}
}

// BenchmarkMemoryPool 基准测试：内存池性能
func BenchmarkMemoryPool(b *testing.B) {
	memManager := ebpf.NewMemoryManager(48 * 1024 * 1024) // 48MB
	defer memManager.Close()

	b.Run("WithPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obj := memManager.GetFromPool("container_metrics")
			if obj != nil {
				memManager.PutToPool("container_metrics", obj)
			}
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = &ebpf.ContainerMetric{}
		}
	})
}

// BenchmarkDataProcessing 基准测试：数据处理性能
func BenchmarkDataProcessing(b *testing.B) {
	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 500,
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		b.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	processor := ebpf.NewDataProcessor(cfg, monitor)
	defer processor.Stop()

	if err := processor.Start(); err != nil {
		b.Fatalf("启动数据处理器失败: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := processor.GetAggregatedMetrics()
		if err != nil {
			b.Fatalf("获取聚合指标失败: %v", err)
		}
	}
}

// BenchmarkContainerSorting 基准测试：容器排序性能
func BenchmarkContainerSorting(b *testing.B) {
	// 创建测试数据
	containers := make([]ebpf.ContainerMetric, 1000)
	for i := range containers {
		containers[i] = ebpf.ContainerMetric{
			ID:             fmt.Sprintf("container_%d", i),
			Name:           fmt.Sprintf("test-container-%d", i),
			CPUPercent:     float64(i % 100),
			MemoryPercent:  float64((i * 2) % 100),
			NetworkLatency: float64((i * 3) % 50),
		}
	}

	b.Run("SortByCPU", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			testContainers := make([]ebpf.ContainerMetric, len(containers))
			copy(testContainers, containers)
			
			sort.Slice(testContainers, func(i, j int) bool {
				return testContainers[i].CPUPercent > testContainers[j].CPUPercent
			})
		}
	})

	b.Run("SortByMemory", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			testContainers := make([]ebpf.ContainerMetric, len(containers))
			copy(testContainers, containers)
			
			sort.Slice(testContainers, func(i, j int) bool {
				return testContainers[i].MemoryPercent > testContainers[j].MemoryPercent
			})
		}
	})
}

// BenchmarkRenderCache 基准测试：渲染缓存性能
func BenchmarkRenderCache(b *testing.B) {
	cache := render.NewRenderCache(100 * time.Millisecond)
	
	// 创建测试指标
	metrics := &ebpf.Metrics{
		Containers: []ebpf.ContainerMetric{
			{ID: "test1", CPUPercent: 50.0},
			{ID: "test2", CPUPercent: 75.0},
		},
		SystemMemory:  1024 * 1024 * 1024,
		EBPFMapsCount: 5,
		LastUpdate:    time.Now(),
	}

	b.Run("CacheHit", func(b *testing.B) {
		// 预填充缓存
		cache.UpdateCache(metrics, nil, nil, nil)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _, hit := cache.GetCachedData(metrics)
			if !hit {
				b.Fatal("缓存未命中")
			}
		}
	})

	b.Run("CacheMiss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// 每次使用不同的指标以确保缓存未命中
			testMetrics := &ebpf.Metrics{
				Containers: []ebpf.ContainerMetric{
					{ID: fmt.Sprintf("test%d", i), CPUPercent: float64(i)},
				},
				LastUpdate: time.Now(),
			}
			_, _, _, hit := cache.GetCachedData(testMetrics)
			if hit {
				b.Fatal("意外的缓存命中")
			}
		}
	})
}

// BenchmarkMemoryUsage 基准测试：内存使用情况
func BenchmarkMemoryUsage(b *testing.B) {
	var m1, m2 runtime.MemStats
	
	// 记录初始内存
	runtime.GC()
	runtime.ReadMemStats(&m1)

	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 1000,
			MemoryLimit:   "48MB",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		monitor, err := ebpf.NewMonitor(cfg)
		if err != nil {
			b.Fatalf("创建监控器失败: %v", err)
		}

		if err := monitor.Start(); err != nil {
			b.Fatalf("启动监控器失败: %v", err)
		}

		// 模拟运行一段时间
		time.Sleep(10 * time.Millisecond)

		monitor.Close()
	}
	b.StopTimer()

	// 记录最终内存
	runtime.GC()
	runtime.ReadMemStats(&m2)

	memoryUsed := m2.Alloc - m1.Alloc
	b.Logf("每次操作平均内存使用: %d bytes", memoryUsed/uint64(b.N))
	
	// 验证内存限制
	maxMemory := uint64(48 * 1024 * 1024) // 48MB
	if m2.Alloc > maxMemory {
		b.Errorf("内存使用超限: %d bytes > %d bytes", m2.Alloc, maxMemory)
	}
}

// BenchmarkConcurrentAccess 基准测试：并发访问性能
func BenchmarkConcurrentAccess(b *testing.B) {
	cfg := &config.Config{
		System: config.SystemConfig{
			MaxContainers: 100,
		},
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		b.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		b.Fatalf("启动监控器失败: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			metrics := monitor.GetMetrics()
			if metrics == nil {
				b.Fatal("获取指标失败")
			}
		}
	})
}

// BenchmarkNetworkMetrics 基准测试：网络指标处理
func BenchmarkNetworkMetrics(b *testing.B) {
	// 创建测试网络数据
	flowKeys := make([]ebpf.FlowKey, 1000)
	flowStats := make([]ebpf.FlowStats, 1000)
	
	for i := range flowKeys {
		flowKeys[i] = ebpf.FlowKey{
			SrcIP:    uint32(i),
			DstIP:    uint32(i + 1000),
			SrcPort:  uint16(i % 65535),
			DstPort:  uint16((i + 1000) % 65535),
			Protocol: 6, // TCP
			CgroupID: uint64(i % 100),
		}
		
		flowStats[i] = ebpf.FlowStats{
			Packets:        uint64(i * 10),
			Bytes:          uint64(i * 1500),
			LatencySum:     uint64(i * 1000000), // 纳秒
			LatencyCount:   uint32(i),
			TCPRetransmits: uint32(i % 10),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 模拟网络指标处理
		for j := range flowKeys {
			_ = flowKeys[j]
			_ = flowStats[j]
		}
	}
}

// 辅助函数
func init() {
	// 设置 GOMAXPROCS 以确保一致的基准测试结果
	runtime.GOMAXPROCS(runtime.NumCPU())
}
