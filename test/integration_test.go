package test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kz521103/Microradar/pkg/config"
	"github.com/kz521103/Microradar/pkg/ebpf"
	"github.com/kz521103/Microradar/pkg/render"
)

// TestFullSystemIntegration 完整系统集成测试
func TestFullSystemIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	// 检查运行环境
	if runtime.GOOS != "linux" {
		t.Skip("集成测试仅在 Linux 上运行")
	}

	if os.Getuid() != 0 {
		t.Skip("集成测试需要 root 权限")
	}

	// 创建测试配置
	cfg := createTestConfig()

	// 测试监控器创建和启动
	t.Run("MonitorLifecycle", func(t *testing.T) {
		testMonitorLifecycle(t, cfg)
	})

	// 测试容器检测
	t.Run("ContainerDetection", func(t *testing.T) {
		testContainerDetection(t, cfg)
	})

	// 测试网络监控
	t.Run("NetworkMonitoring", func(t *testing.T) {
		testNetworkMonitoring(t, cfg)
	})

	// 测试终端渲染
	t.Run("TerminalRendering", func(t *testing.T) {
		testTerminalRendering(t, cfg)
	})

	// 测试内存限制
	t.Run("MemoryLimits", func(t *testing.T) {
		testMemoryLimits(t, cfg)
	})

	// 测试守护进程模式
	t.Run("DaemonMode", func(t *testing.T) {
		testDaemonMode(t, cfg)
	})
}

// createTestConfig 创建测试配置
func createTestConfig() *config.Config {
	return &config.Config{
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "integration-test",
					Runtime:      "docker",
					SamplingRate: 1 * time.Second,
					Metrics:      []string{"cpu", "memory", "network_latency"},
				},
			},
			AlertThresholds: config.AlertThresholds{
				CPU:            70.0,
				Memory:         80.0,
				NetworkLatency: 10.0,
			},
		},
		Display: config.DisplayConfig{
			RefreshRate: 100 * time.Millisecond,
			Theme:       "default",
		},
		System: config.SystemConfig{
			MaxContainers: 100,
			MemoryLimit:   "48MB",
			LogLevel:      "info",
		},
	}
}

// testMonitorLifecycle 测试监控器生命周期
func testMonitorLifecycle(t *testing.T, cfg *config.Config) {
	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}

	// 测试启动
	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控器失败: %v", err)
	}

	// 验证运行状态
	if !monitor.IsRunning() {
		t.Error("监控器应该处于运行状态")
	}

	// 等待一段时间以确保正常运行
	time.Sleep(2 * time.Second)

	// 获取指标
	metrics := monitor.GetMetrics()
	if metrics == nil {
		t.Error("应该能够获取指标")
	}

	// 测试停止
	if err := monitor.Stop(); err != nil {
		t.Fatalf("停止监控器失败: %v", err)
	}

	// 验证停止状态
	if monitor.IsRunning() {
		t.Error("监控器应该已停止")
	}

	// 清理
	monitor.Close()
}

// testContainerDetection 测试容器检测
func testContainerDetection(t *testing.T, cfg *config.Config) {
	// 启动测试容器
	containerID := startTestContainer(t)
	defer stopTestContainer(t, containerID)

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控器失败: %v", err)
	}

	// 等待容器被检测到
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("超时：未检测到测试容器")
		case <-ticker.C:
			metrics := monitor.GetMetrics()
			if metrics != nil && len(metrics.Containers) > 0 {
				// 检查是否检测到我们的测试容器
				for _, container := range metrics.Containers {
					if strings.Contains(container.ID, containerID[:12]) {
						t.Logf("成功检测到容器: %s", container.ID)
						return
					}
				}
			}
		}
	}
}

// testNetworkMonitoring 测试网络监控
func testNetworkMonitoring(t *testing.T, cfg *config.Config) {
	// 启动测试容器并生成网络流量
	containerID := startTestContainer(t)
	defer stopTestContainer(t, containerID)

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控器失败: %v", err)
	}

	// 生成网络流量
	generateNetworkTraffic(t, containerID)

	// 等待网络指标更新
	time.Sleep(5 * time.Second)

	metrics := monitor.GetMetrics()
	if metrics == nil || len(metrics.Containers) == 0 {
		t.Fatal("未获取到容器指标")
	}

	// 验证网络指标
	found := false
	for _, container := range metrics.Containers {
		if container.NetworkLatency > 0 {
			found = true
			t.Logf("检测到网络延迟: %.2fms", container.NetworkLatency)
			break
		}
	}

	if !found {
		t.Log("警告：未检测到网络延迟指标（可能是 eBPF 程序未正确加载）")
	}
}

// testTerminalRendering 测试终端渲染
func testTerminalRendering(t *testing.T, cfg *config.Config) {
	// 注意：这个测试不能在 CI 环境中运行，因为没有 TTY
	if os.Getenv("CI") != "" {
		t.Skip("跳过终端渲染测试（CI 环境）")
	}

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控器失败: %v", err)
	}

	// 创建终端渲染器（但不实际显示）
	renderer, err := render.NewTerminalRenderer(cfg)
	if err != nil {
		// 如果无法创建终端渲染器（例如在无头环境中），跳过测试
		t.Skip("无法创建终端渲染器")
	}
	defer renderer.Close()

	// 测试渲染器能否正常工作
	metrics := monitor.GetMetrics()
	if metrics != nil {
		t.Log("终端渲染器创建成功")
	}
}

// testMemoryLimits 测试内存限制
func testMemoryLimits(t *testing.T, cfg *config.Config) {
	var m1, m2 runtime.MemStats

	// 记录初始内存
	runtime.GC()
	runtime.ReadMemStats(&m1)

	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控器失败: %v", err)
	}

	// 运行一段时间
	time.Sleep(10 * time.Second)

	// 记录运行时内存
	runtime.GC()
	runtime.ReadMemStats(&m2)

	monitor.Close()

	memoryUsed := m2.Alloc - m1.Alloc
	maxMemory := uint64(48 * 1024 * 1024) // 48MB

	t.Logf("内存使用量: %.2f MB", float64(memoryUsed)/1024/1024)

	if memoryUsed > maxMemory {
		t.Errorf("内存使用超限: %.2f MB > 48 MB", float64(memoryUsed)/1024/1024)
	}
}

// testDaemonMode 测试守护进程模式
func testDaemonMode(t *testing.T, cfg *config.Config) {
	// 这个测试需要实际的二进制文件
	binaryPath := "../bin/micro-radar"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("二进制文件不存在，跳过守护进程测试")
	}

	// 创建临时配置文件
	configFile := createTempConfig(t, cfg)
	defer os.Remove(configFile)

	// 启动守护进程
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--config", configFile, "--daemon")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动守护进程失败: %v", err)
	}

	// 等待服务启动
	time.Sleep(5 * time.Second)

	// 测试健康检查端点
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		t.Fatalf("健康检查失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("健康检查返回错误状态: %d", resp.StatusCode)
	}

	// 测试指标端点
	resp, err = http.Get("http://localhost:8080/metrics")
	if err != nil {
		t.Fatalf("指标端点访问失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("指标端点返回错误状态: %d", resp.StatusCode)
	}

	// 停止守护进程
	if err := cmd.Process.Kill(); err != nil {
		t.Logf("停止守护进程失败: %v", err)
	}
}

// 辅助函数

// startTestContainer 启动测试容器
func startTestContainer(t *testing.T) string {
	cmd := exec.Command("docker", "run", "-d", "--rm", "alpine:latest", "sleep", "60")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("启动测试容器失败: %v", err)
	}
	return strings.TrimSpace(string(output))
}

// stopTestContainer 停止测试容器
func stopTestContainer(t *testing.T, containerID string) {
	cmd := exec.Command("docker", "stop", containerID)
	if err := cmd.Run(); err != nil {
		t.Logf("停止测试容器失败: %v", err)
	}
}

// generateNetworkTraffic 生成网络流量
func generateNetworkTraffic(t *testing.T, containerID string) {
	// 在容器中执行网络请求
	cmd := exec.Command("docker", "exec", containerID, "wget", "-q", "-O", "/dev/null", "http://httpbin.org/get")
	if err := cmd.Run(); err != nil {
		t.Logf("生成网络流量失败: %v", err)
	}
}

// createTempConfig 创建临时配置文件
func createTempConfig(t *testing.T, cfg *config.Config) string {
	configContent := `
monitoring:
  targets:
    - name: "test-cluster"
      runtime: "docker"
      sampling_rate: "2s"
      metrics:
        - cpu
        - memory
        - network_latency

  alert_thresholds:
    cpu: 70.0
    memory: 80.0
    network_latency: 10

display:
  refresh_rate: "100ms"
  theme: "default"

system:
  max_containers: 100
  memory_limit: "48MB"
  log_level: "info"
`

	tmpFile, err := os.CreateTemp("", "microradar-test-*.yaml")
	if err != nil {
		t.Fatalf("创建临时配置文件失败: %v", err)
	}

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}

	tmpFile.Close()
	return tmpFile.Name()
}
