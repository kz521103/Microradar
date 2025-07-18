package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RuntimeDetector 容器运行时检测器
type RuntimeDetector struct {
	detectedRuntimes []RuntimeInfo
	lastScan         time.Time
}

// RuntimeInfo 运行时信息
type RuntimeInfo struct {
	Name       string `json:"name"`        // docker, containerd, cri-o
	Version    string `json:"version"`     // 版本号
	SocketPath string `json:"socket_path"` // Socket 路径
	Available  bool   `json:"available"`   // 是否可用
	PID        int    `json:"pid"`         // 进程 PID
}

// ContainerInfo 容器信息
type ContainerRuntimeInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Status      string            `json:"status"`
	Runtime     string            `json:"runtime"`
	CgroupPath  string            `json:"cgroup_path"`
	CgroupID    uint64            `json:"cgroup_id"`
	PID         int               `json:"pid"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   time.Time         `json:"started_at"`
}

// NewRuntimeDetector 创建运行时检测器
func NewRuntimeDetector() *RuntimeDetector {
	return &RuntimeDetector{
		detectedRuntimes: make([]RuntimeInfo, 0),
	}
}

// DetectRuntimes 检测可用的容器运行时
func (rd *RuntimeDetector) DetectRuntimes() ([]RuntimeInfo, error) {
	// 如果最近扫描过，返回缓存结果
	if time.Since(rd.lastScan) < 30*time.Second {
		return rd.detectedRuntimes, nil
	}

	var runtimes []RuntimeInfo

	// 检测 Docker
	if dockerInfo := rd.detectDocker(); dockerInfo.Available {
		runtimes = append(runtimes, dockerInfo)
	}

	// 检测 containerd
	if containerdInfo := rd.detectContainerd(); containerdInfo.Available {
		runtimes = append(runtimes, containerdInfo)
	}

	// 检测 CRI-O
	if crioInfo := rd.detectCRIO(); crioInfo.Available {
		runtimes = append(runtimes, crioInfo)
	}

	rd.detectedRuntimes = runtimes
	rd.lastScan = time.Now()

	return runtimes, nil
}

// detectDocker 检测 Docker
func (rd *RuntimeDetector) detectDocker() RuntimeInfo {
	info := RuntimeInfo{
		Name:      "docker",
		Available: false,
	}

	// 检查 Docker socket
	socketPaths := []string{
		"/var/run/docker.sock",
		"/run/docker.sock",
	}

	for _, path := range socketPaths {
		if _, err := os.Stat(path); err == nil {
			info.SocketPath = path
			info.Available = true
			break
		}
	}

	// 检查 Docker 进程
	if pid := rd.findProcessByName("dockerd"); pid > 0 {
		info.PID = pid
		info.Available = true
	}

	// 获取版本信息
	if info.Available {
		info.Version = rd.getDockerVersion()
	}

	return info
}

// detectContainerd 检测 containerd
func (rd *RuntimeDetector) detectContainerd() RuntimeInfo {
	info := RuntimeInfo{
		Name:      "containerd",
		Available: false,
	}

	// 检查 containerd socket
	socketPaths := []string{
		"/run/containerd/containerd.sock",
		"/var/run/containerd/containerd.sock",
	}

	for _, path := range socketPaths {
		if _, err := os.Stat(path); err == nil {
			info.SocketPath = path
			info.Available = true
			break
		}
	}

	// 检查 containerd 进程
	if pid := rd.findProcessByName("containerd"); pid > 0 {
		info.PID = pid
		info.Available = true
	}

	// 获取版本信息
	if info.Available {
		info.Version = rd.getContainerdVersion()
	}

	return info
}

// detectCRIO 检测 CRI-O
func (rd *RuntimeDetector) detectCRIO() RuntimeInfo {
	info := RuntimeInfo{
		Name:      "cri-o",
		Available: false,
	}

	// 检查 CRI-O socket
	socketPaths := []string{
		"/var/run/crio/crio.sock",
		"/run/crio/crio.sock",
	}

	for _, path := range socketPaths {
		if _, err := os.Stat(path); err == nil {
			info.SocketPath = path
			info.Available = true
			break
		}
	}

	// 检查 CRI-O 进程
	if pid := rd.findProcessByName("crio"); pid > 0 {
		info.PID = pid
		info.Available = true
	}

	// 获取版本信息
	if info.Available {
		info.Version = rd.getCRIOVersion()
	}

	return info
}

// findProcessByName 根据进程名查找 PID
func (rd *RuntimeDetector) findProcessByName(name string) int {
	procDir := "/proc"
	
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// 检查是否为数字目录名（PID）
		pid := 0
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil {
			continue
		}

		// 读取进程命令行
		cmdlinePath := filepath.Join(procDir, entry.Name(), "cmdline")
		cmdlineBytes, err := os.ReadFile(cmdlinePath)
		if err != nil {
			continue
		}

		cmdline := string(cmdlineBytes)
		if strings.Contains(cmdline, name) {
			return pid
		}
	}

	return 0
}

// getDockerVersion 获取 Docker 版本
func (rd *RuntimeDetector) getDockerVersion() string {
	// 尝试从 /proc/version 或其他方式获取版本
	// 这里简化实现
	return "unknown"
}

// getContainerdVersion 获取 containerd 版本
func (rd *RuntimeDetector) getContainerdVersion() string {
	// 尝试从配置文件或进程信息获取版本
	return "unknown"
}

// getCRIOVersion 获取 CRI-O 版本
func (rd *RuntimeDetector) getCRIOVersion() string {
	// 尝试从配置文件获取版本
	return "unknown"
}

// GetContainers 获取容器列表
func (rd *RuntimeDetector) GetContainers(runtime string) ([]ContainerRuntimeInfo, error) {
	switch runtime {
	case "docker":
		return rd.getDockerContainers()
	case "containerd":
		return rd.getContainerdContainers()
	case "cri-o":
		return rd.getCRIOContainers()
	default:
		return nil, fmt.Errorf("不支持的运行时: %s", runtime)
	}
}

// getDockerContainers 获取 Docker 容器
func (rd *RuntimeDetector) getDockerContainers() ([]ContainerRuntimeInfo, error) {
	var containers []ContainerRuntimeInfo

	// 从 /proc/*/cgroup 文件中解析容器信息
	containers = append(containers, rd.parseContainersFromCgroup("docker")...)

	return containers, nil
}

// getContainerdContainers 获取 containerd 容器
func (rd *RuntimeDetector) getContainerdContainers() ([]ContainerRuntimeInfo, error) {
	var containers []ContainerRuntimeInfo

	// 从 cgroup 解析 containerd 容器
	containers = append(containers, rd.parseContainersFromCgroup("containerd")...)

	return containers, nil
}

// getCRIOContainers 获取 CRI-O 容器
func (rd *RuntimeDetector) getCRIOContainers() ([]ContainerRuntimeInfo, error) {
	var containers []ContainerRuntimeInfo

	// 从 cgroup 解析 CRI-O 容器
	containers = append(containers, rd.parseContainersFromCgroup("crio")...)

	return containers, nil
}

// parseContainersFromCgroup 从 cgroup 解析容器信息
func (rd *RuntimeDetector) parseContainersFromCgroup(runtime string) []ContainerRuntimeInfo {
	var containers []ContainerRuntimeInfo

	// 遍历 /proc/*/cgroup 文件
	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return containers
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid := 0
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil {
			continue
		}

		cgroupPath := filepath.Join(procDir, entry.Name(), "cgroup")
		if container := rd.parseCgroupFile(cgroupPath, runtime, pid); container != nil {
			containers = append(containers, *container)
		}
	}

	return containers
}

// parseCgroupFile 解析 cgroup 文件
func (rd *RuntimeDetector) parseCgroupFile(cgroupPath, runtime string, pid int) *ContainerRuntimeInfo {
	file, err := os.Open(cgroupPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		
		// 检查是否包含容器运行时信息
		if rd.isContainerCgroup(line, runtime) {
			containerID := rd.extractContainerID(line, runtime)
			if containerID != "" {
				return &ContainerRuntimeInfo{
					ID:         containerID,
					Runtime:    runtime,
					PID:        pid,
					CgroupPath: line,
					Status:     "running",
					CreatedAt:  time.Now(), // 简化实现
					StartedAt:  time.Now(),
				}
			}
		}
	}

	return nil
}

// isContainerCgroup 检查是否为容器 cgroup
func (rd *RuntimeDetector) isContainerCgroup(line, runtime string) bool {
	switch runtime {
	case "docker":
		return strings.Contains(line, "/docker/") || strings.Contains(line, "/docker-")
	case "containerd":
		return strings.Contains(line, "/containerd/") || strings.Contains(line, "/k8s.io/")
	case "cri-o":
		return strings.Contains(line, "/crio-") || strings.Contains(line, "/crio/")
	}
	return false
}

// extractContainerID 提取容器 ID
func (rd *RuntimeDetector) extractContainerID(line, runtime string) string {
	switch runtime {
	case "docker":
		// Docker cgroup 格式: /docker/container_id
		parts := strings.Split(line, "/docker/")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	case "containerd":
		// containerd cgroup 格式: /containerd/container_id
		parts := strings.Split(line, "/containerd/")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	case "cri-o":
		// CRI-O cgroup 格式: /crio-container_id
		parts := strings.Split(line, "/crio-")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}
