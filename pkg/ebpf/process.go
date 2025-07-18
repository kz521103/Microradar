package ebpf

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ProcessManager 进程管理器
type ProcessManager struct {
	monitor *Monitor
}

// NewProcessManager 创建进程管理器
func NewProcessManager(monitor *Monitor) *ProcessManager {
	return &ProcessManager{
		monitor: monitor,
	}
}

// KillContainerProcess 取消容器进程
func (pm *ProcessManager) KillContainerProcess(containerIndex int) error {
	metrics := pm.monitor.GetMetrics()
	if metrics == nil || containerIndex >= len(metrics.Containers) || containerIndex < 0 {
		return fmt.Errorf("无效的容器索引: %d", containerIndex)
	}

	container := metrics.Containers[containerIndex]
	
	// 根据容器运行时选择不同的取消策略
	runtime := pm.detectContainerRuntime(container.ID)
	
	switch runtime {
	case "docker":
		return pm.killDockerContainer(container.ID)
	case "containerd":
		return pm.killContainerdContainer(container.ID)
	case "cri-o":
		return pm.killCRIOContainer(container.ID)
	default:
		// 直接取消进程
		return pm.killProcessByPID(container.PID)
	}
}

// detectContainerRuntime 检测容器运行时
func (pm *ProcessManager) detectContainerRuntime(containerID string) string {
	// 检查 Docker
	if pm.isDockerContainer(containerID) {
		return "docker"
	}
	
	// 检查 containerd
	if pm.isContainerdContainer(containerID) {
		return "containerd"
	}
	
	// 检查 CRI-O
	if pm.isCRIOContainer(containerID) {
		return "cri-o"
	}
	
	return "unknown"
}

// isDockerContainer 检查是否为 Docker 容器
func (pm *ProcessManager) isDockerContainer(containerID string) bool {
	cmd := exec.Command("docker", "inspect", containerID)
	return cmd.Run() == nil
}

// isContainerdContainer 检查是否为 containerd 容器
func (pm *ProcessManager) isContainerdContainer(containerID string) bool {
	cmd := exec.Command("ctr", "container", "info", containerID)
	return cmd.Run() == nil
}

// isCRIOContainer 检查是否为 CRI-O 容器
func (pm *ProcessManager) isCRIOContainer(containerID string) bool {
	cmd := exec.Command("crictl", "inspect", containerID)
	return cmd.Run() == nil
}

// killDockerContainer 取消 Docker 容器
func (pm *ProcessManager) killDockerContainer(containerID string) error {
	// 首先尝试优雅停止
	cmd := exec.Command("docker", "stop", "--time", "10", containerID)
	if err := cmd.Run(); err == nil {
		return nil
	}
	
	// 如果优雅停止失败，强制取消
	cmd = exec.Command("docker", "kill", containerID)
	return cmd.Run()
}

// killContainerdContainer 取消 containerd 容器
func (pm *ProcessManager) killContainerdContainer(containerID string) error {
	// 首先尝试优雅停止
	cmd := exec.Command("ctr", "task", "kill", "--signal", "SIGTERM", containerID)
	if err := cmd.Run(); err == nil {
		// 等待一段时间
		time.Sleep(10 * time.Second)
		
		// 检查是否已停止
		cmd = exec.Command("ctr", "task", "ls")
		output, err := cmd.Output()
		if err == nil && !strings.Contains(string(output), containerID) {
			return nil
		}
	}
	
	// 强制取消
	cmd = exec.Command("ctr", "task", "kill", "--signal", "SIGKILL", containerID)
	return cmd.Run()
}

// killCRIOContainer 取消 CRI-O 容器
func (pm *ProcessManager) killCRIOContainer(containerID string) error {
	// 首先尝试优雅停止
	cmd := exec.Command("crictl", "stop", containerID)
	if err := cmd.Run(); err == nil {
		return nil
	}
	
	// 强制取消
	cmd = exec.Command("crictl", "rm", "--force", containerID)
	return cmd.Run()
}

// killProcessByPID 通过 PID 取消进程
func (pm *ProcessManager) killProcessByPID(pid uint32) error {
	if pid == 0 {
		return fmt.Errorf("无效的 PID: %d", pid)
	}
	
	// 首先尝试 SIGTERM
	if err := pm.sendSignal(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("发送 SIGTERM 失败: %w", err)
	}
	
	// 等待进程退出
	time.Sleep(5 * time.Second)
	
	// 检查进程是否还存在
	if pm.isProcessRunning(pid) {
		// 发送 SIGKILL
		if err := pm.sendSignal(pid, syscall.SIGKILL); err != nil {
			return fmt.Errorf("发送 SIGKILL 失败: %w", err)
		}
	}
	
	return nil
}

// sendSignal 发送信号给进程
func (pm *ProcessManager) sendSignal(pid uint32, signal syscall.Signal) error {
	process, err := os.FindProcess(int(pid))
	if err != nil {
		return fmt.Errorf("查找进程失败: %w", err)
	}
	
	return process.Signal(signal)
}

// isProcessRunning 检查进程是否在运行
func (pm *ProcessManager) isProcessRunning(pid uint32) bool {
	process, err := os.FindProcess(int(pid))
	if err != nil {
		return false
	}
	
	// 发送信号 0 来检查进程是否存在
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// GetProcessInfo 获取进程信息
func (pm *ProcessManager) GetProcessInfo(pid uint32) (*ProcessInfo, error) {
	if pid == 0 {
		return nil, fmt.Errorf("无效的 PID: %d", pid)
	}
	
	// 读取 /proc/PID/stat 文件
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return nil, fmt.Errorf("读取进程状态失败: %w", err)
	}
	
	// 解析状态信息
	fields := strings.Fields(string(statData))
	if len(fields) < 24 {
		return nil, fmt.Errorf("进程状态格式无效")
	}
	
	// 解析各个字段
	ppid, _ := strconv.ParseUint(fields[3], 10, 32)
	state := fields[2]
	
	// 读取进程名
	commPath := fmt.Sprintf("/proc/%d/comm", pid)
	commData, err := os.ReadFile(commPath)
	if err != nil {
		return nil, fmt.Errorf("读取进程名失败: %w", err)
	}
	
	processInfo := &ProcessInfo{
		PID:   pid,
		PPID:  uint32(ppid),
		Name:  strings.TrimSpace(string(commData)),
		State: state,
	}
	
	return processInfo, nil
}

// ProcessInfo 进程信息
type ProcessInfo struct {
	PID   uint32 `json:"pid"`
	PPID  uint32 `json:"ppid"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// KillProcessOptions 取消进程选项
type KillProcessOptions struct {
	Force         bool          `json:"force"`          // 是否强制取消
	GracePeriod   time.Duration `json:"grace_period"`   // 优雅停止等待时间
	RemoveVolumes bool          `json:"remove_volumes"` // 是否删除卷
	RemoveLinks   bool          `json:"remove_links"`   // 是否删除链接
}

// KillContainerWithOptions 使用选项取消容器
func (pm *ProcessManager) KillContainerWithOptions(containerIndex int, options KillProcessOptions) error {
	metrics := pm.monitor.GetMetrics()
	if metrics == nil || containerIndex >= len(metrics.Containers) || containerIndex < 0 {
		return fmt.Errorf("无效的容器索引: %d", containerIndex)
	}

	container := metrics.Containers[containerIndex]
	runtime := pm.detectContainerRuntime(container.ID)
	
	switch runtime {
	case "docker":
		return pm.killDockerContainerWithOptions(container.ID, options)
	case "containerd":
		return pm.killContainerdContainerWithOptions(container.ID, options)
	case "cri-o":
		return pm.killCRIOContainerWithOptions(container.ID, options)
	default:
		return pm.killProcessByPIDWithOptions(container.PID, options)
	}
}

// killDockerContainerWithOptions 使用选项取消 Docker 容器
func (pm *ProcessManager) killDockerContainerWithOptions(containerID string, options KillProcessOptions) error {
	if options.Force {
		cmd := exec.Command("docker", "kill", containerID)
		return cmd.Run()
	}
	
	// 优雅停止
	timeout := int(options.GracePeriod.Seconds())
	if timeout <= 0 {
		timeout = 10
	}
	
	cmd := exec.Command("docker", "stop", "--time", strconv.Itoa(timeout), containerID)
	return cmd.Run()
}

// killContainerdContainerWithOptions 使用选项取消 containerd 容器
func (pm *ProcessManager) killContainerdContainerWithOptions(containerID string, options KillProcessOptions) error {
	signal := "SIGTERM"
	if options.Force {
		signal = "SIGKILL"
	}
	
	cmd := exec.Command("ctr", "task", "kill", "--signal", signal, containerID)
	return cmd.Run()
}

// killCRIOContainerWithOptions 使用选项取消 CRI-O 容器
func (pm *ProcessManager) killCRIOContainerWithOptions(containerID string, options KillProcessOptions) error {
	if options.Force {
		cmd := exec.Command("crictl", "rm", "--force", containerID)
		return cmd.Run()
	}
	
	cmd := exec.Command("crictl", "stop", containerID)
	return cmd.Run()
}

// killProcessByPIDWithOptions 使用选项通过 PID 取消进程
func (pm *ProcessManager) killProcessByPIDWithOptions(pid uint32, options KillProcessOptions) error {
	if options.Force {
		return pm.sendSignal(pid, syscall.SIGKILL)
	}
	
	// 发送 SIGTERM
	if err := pm.sendSignal(pid, syscall.SIGTERM); err != nil {
		return err
	}
	
	// 等待优雅停止
	gracePeriod := options.GracePeriod
	if gracePeriod <= 0 {
		gracePeriod = 10 * time.Second
	}
	
	time.Sleep(gracePeriod)
	
	// 检查是否还在运行
	if pm.isProcessRunning(pid) {
		return pm.sendSignal(pid, syscall.SIGKILL)
	}
	
	return nil
}
