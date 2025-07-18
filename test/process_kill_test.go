package test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kz521103/Microradar/pkg/config"
	"github.com/kz521103/Microradar/pkg/ebpf"
)

// TestProcessKillFunctionality 测试进程取消功能
func TestProcessKillFunctionality(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过进程取消测试")
	}

	// 检查是否有 Docker
	if !isDockerAvailable() {
		t.Skip("Docker 不可用，跳过测试")
	}

	cfg := createTestConfig()
	
	// 创建监控器
	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	if err := monitor.Start(); err != nil {
		t.Fatalf("启动监控器失败: %v", err)
	}

	// 启动测试容器
	containerID := startLongRunningContainer(t)
	defer cleanupContainer(t, containerID)

	// 等待容器被检测到
	waitForContainerDetection(t, monitor, containerID, 30*time.Second)

	// 测试进程取消
	t.Run("KillContainerProcess", func(t *testing.T) {
		testKillContainerProcess(t, monitor, containerID)
	})

	// 测试带选项的进程取消
	t.Run("KillContainerProcessWithOptions", func(t *testing.T) {
		testKillContainerProcessWithOptions(t, monitor)
	})
}

// isDockerAvailable 检查 Docker 是否可用
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

// startLongRunningContainer 启动长时间运行的测试容器
func startLongRunningContainer(t *testing.T) string {
	cmd := exec.Command("docker", "run", "-d", "--rm", "--name", "microradar-kill-test", 
		"alpine:latest", "sh", "-c", "while true; do sleep 1; done")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("启动测试容器失败: %v", err)
	}
	return strings.TrimSpace(string(output))
}

// cleanupContainer 清理测试容器
func cleanupContainer(t *testing.T, containerID string) {
	cmd := exec.Command("docker", "rm", "-f", containerID)
	if err := cmd.Run(); err != nil {
		t.Logf("清理容器失败: %v", err)
	}
}

// waitForContainerDetection 等待容器被检测到
func waitForContainerDetection(t *testing.T, monitor *ebpf.Monitor, containerID string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("超时：未检测到测试容器")
		case <-ticker.C:
			metrics := monitor.GetMetrics()
			if metrics != nil {
				for _, container := range metrics.Containers {
					if strings.Contains(container.ID, containerID[:12]) {
						t.Logf("检测到容器: %s", container.ID)
						return
					}
				}
			}
		}
	}
}

// testKillContainerProcess 测试基本的进程取消功能
func testKillContainerProcess(t *testing.T, monitor *ebpf.Monitor, containerID string) {
	// 获取当前指标
	metrics := monitor.GetMetrics()
	if metrics == nil || len(metrics.Containers) == 0 {
		t.Fatal("未找到容器指标")
	}

	// 找到测试容器的索引
	containerIndex := -1
	for i, container := range metrics.Containers {
		if strings.Contains(container.ID, containerID[:12]) {
			containerIndex = i
			break
		}
	}

	if containerIndex == -1 {
		t.Fatal("未找到测试容器")
	}

	// 验证容器正在运行
	if !isContainerRunning(containerID) {
		t.Fatal("测试容器未运行")
	}

	// 取消容器进程
	err := monitor.KillContainerProcess(containerIndex)
	if err != nil {
		t.Fatalf("取消容器进程失败: %v", err)
	}

	// 等待容器停止
	waitForContainerStop(t, containerID, 15*time.Second)

	// 验证容器已停止
	if isContainerRunning(containerID) {
		t.Error("容器应该已经停止")
	}
}

// testKillContainerProcessWithOptions 测试带选项的进程取消
func testKillContainerProcessWithOptions(t *testing.T, monitor *ebpf.Monitor) {
	// 启动另一个测试容器
	containerID := startLongRunningContainer(t)
	defer cleanupContainer(t, containerID)

	// 等待容器被检测到
	waitForContainerDetection(t, monitor, containerID, 30*time.Second)

	// 获取容器索引
	metrics := monitor.GetMetrics()
	containerIndex := -1
	for i, container := range metrics.Containers {
		if strings.Contains(container.ID, containerID[:12]) {
			containerIndex = i
			break
		}
	}

	if containerIndex == -1 {
		t.Fatal("未找到测试容器")
	}

	// 使用选项取消进程
	options := ebpf.KillProcessOptions{
		Force:       false,
		GracePeriod: 5 * time.Second,
	}

	err := monitor.KillContainerProcessWithOptions(containerIndex, options)
	if err != nil {
		t.Fatalf("使用选项取消容器进程失败: %v", err)
	}

	// 等待容器停止
	waitForContainerStop(t, containerID, 10*time.Second)

	// 验证容器已停止
	if isContainerRunning(containerID) {
		t.Error("容器应该已经停止")
	}
}

// isContainerRunning 检查容器是否正在运行
func isContainerRunning(containerID string) bool {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// waitForContainerStop 等待容器停止
func waitForContainerStop(t *testing.T, containerID string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Log("等待容器停止超时")
			return
		case <-ticker.C:
			if !isContainerRunning(containerID) {
				t.Log("容器已停止")
				return
			}
		}
	}
}

// TestProcessManagerDirectly 直接测试进程管理器
func TestProcessManagerDirectly(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过进程管理器直接测试")
	}

	cfg := createTestConfig()
	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	processManager := monitor.GetProcessManager()
	if processManager == nil {
		t.Fatal("进程管理器不应为空")
	}

	// 测试无效索引
	err = processManager.KillContainerProcess(-1)
	if err == nil {
		t.Error("应该返回错误：无效的容器索引")
	}

	err = processManager.KillContainerProcess(999)
	if err == nil {
		t.Error("应该返回错误：容器索引超出范围")
	}
}

// TestProcessInfo 测试进程信息获取
func TestProcessInfo(t *testing.T) {
	cfg := createTestConfig()
	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		t.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	processManager := monitor.GetProcessManager()

	// 测试获取当前进程信息
	processInfo, err := processManager.GetProcessInfo(uint32(1)) // init 进程
	if err != nil {
		t.Fatalf("获取进程信息失败: %v", err)
	}

	if processInfo.PID != 1 {
		t.Errorf("期望 PID 为 1，实际为 %d", processInfo.PID)
	}

	if processInfo.Name == "" {
		t.Error("进程名不应为空")
	}

	t.Logf("进程信息: PID=%d, PPID=%d, Name=%s, State=%s", 
		processInfo.PID, processInfo.PPID, processInfo.Name, processInfo.State)

	// 测试无效 PID
	_, err = processManager.GetProcessInfo(0)
	if err == nil {
		t.Error("应该返回错误：无效的 PID")
	}

	_, err = processManager.GetProcessInfo(999999)
	if err == nil {
		t.Error("应该返回错误：进程不存在")
	}
}

// createTestConfig 创建测试配置
func createTestConfig() *config.Config {
	return &config.Config{
		Monitoring: config.MonitoringConfig{
			Targets: []config.TargetConfig{
				{
					Name:         "kill-test",
					Runtime:      "docker",
					SamplingRate: 1 * time.Second,
				},
			},
		},
		System: config.SystemConfig{
			MaxContainers: 100,
			MemoryLimit:   "48MB",
		},
	}
}
