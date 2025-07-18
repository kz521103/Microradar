package ebpf

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"

	"github.com/kz521103/Microradar/pkg/config"
)

// eBPF 数据结构定义 (与 C 结构体对应)

// ContainerInfo 容器信息结构 (对应 C 的 container_info)
type ContainerInfo struct {
	CgroupID    uint64     `json:"cgroup_id"`
	PID         uint32     `json:"pid"`
	PPID        uint32     `json:"ppid"`
	ContainerID [64]byte   `json:"container_id"`
	Comm        [16]byte   `json:"comm"`
	StartTime   uint64     `json:"start_time"`
	CPUUsage    uint32     `json:"cpu_usage"`
	MemoryUsage uint64     `json:"memory_usage"`
	Status      uint32     `json:"status"`
}

// FlowKey 网络流量键 (对应 C 的 flow_key)
type FlowKey struct {
	SrcIP     uint32 `json:"src_ip"`
	DstIP     uint32 `json:"dst_ip"`
	SrcPort   uint16 `json:"src_port"`
	DstPort   uint16 `json:"dst_port"`
	Protocol  uint8  `json:"protocol"`
	Pad       [3]byte `json:"pad"`
	CgroupID  uint64 `json:"cgroup_id"`
}

// FlowStats 网络流量统计 (对应 C 的 flow_stats)
type FlowStats struct {
	Packets        uint64 `json:"packets"`
	Bytes          uint64 `json:"bytes"`
	LatencySum     uint64 `json:"latency_sum"`
	LatencyCount   uint32 `json:"latency_count"`
	LastSeen       uint64 `json:"last_seen"`
	TCPRetransmits uint32 `json:"tcp_retransmits"`
	Flags          uint32 `json:"flags"`
}

// EventData 事件数据结构 (对应 C 的 event_data)
type EventData struct {
	Type      uint32 `json:"type"`
	Timestamp uint64 `json:"timestamp"`
	CgroupID  uint64 `json:"cgroup_id"`
	PID       uint32 `json:"pid"`
	Data      [256]byte `json:"data"` // 联合体数据
}

// Monitor eBPF 监控器
type Monitor struct {
	config          *config.Config
	spec            *ebpf.CollectionSpec
	coll            *ebpf.Collection
	links           []link.Link
	running         bool
	startTime       time.Time
	metrics         *Metrics
	mu              sync.RWMutex

	// 数据处理引擎
	processor       *DataProcessor

	// 运行时检测器
	runtimeDetector *RuntimeDetector

	// 进程管理器
	processManager  *ProcessManager
}

// Metrics 监控指标
type Metrics struct {
	Containers     []ContainerMetric `json:"containers"`
	SystemMemory   uint64           `json:"system_memory"`
	EBPFMapsCount  int              `json:"ebpf_maps_count"`
	LastUpdate     time.Time        `json:"last_update"`
}

// ContainerMetric 容器指标
type ContainerMetric struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PID            uint32    `json:"pid"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryPercent  float64   `json:"memory_percent"`
	MemoryUsage    uint64    `json:"memory_usage"`
	NetworkLatency float64   `json:"network_latency"`
	TCPRetransmits uint32    `json:"tcp_retransmits"`
	Status         string    `json:"status"`
	StartTime      time.Time `json:"start_time"`
}

// NewMonitor 创建新的 eBPF 监控器
func NewMonitor(cfg *config.Config) (*Monitor, error) {
	// 移除内存限制 (需要 CAP_SYS_RESOURCE 或 root)
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("移除内存限制失败: %w", err)
	}

	monitor := &Monitor{
		config:          cfg,
		metrics: &Metrics{
			Containers: make([]ContainerMetric, 0),
		},
		runtimeDetector: NewRuntimeDetector(),
	}

	// 创建数据处理引擎
	monitor.processor = NewDataProcessor(cfg, monitor)

	// 创建进程管理器
	monitor.processManager = NewProcessManager(monitor)

	return monitor, nil
}

// Start 启动监控
func (m *Monitor) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("监控器已在运行")
	}

	// 加载 eBPF 程序
	if err := m.loadPrograms(); err != nil {
		return fmt.Errorf("加载 eBPF 程序失败: %w", err)
	}

	// 附加到内核
	if err := m.attachPrograms(); err != nil {
		m.cleanup()
		return fmt.Errorf("附加 eBPF 程序失败: %w", err)
	}

	// 启动数据处理引擎
	if err := m.processor.Start(); err != nil {
		m.cleanup()
		return fmt.Errorf("启动数据处理引擎失败: %w", err)
	}

	// 启动数据收集
	go m.collectData()

	m.running = true
	m.startTime = time.Now()

	return nil
}

// Stop 停止监控
func (m *Monitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	// 停止数据处理引擎
	if m.processor != nil {
		m.processor.Stop()
	}

	m.cleanup()
	m.running = false

	return nil
}

// Close 关闭监控器
func (m *Monitor) Close() error {
	return m.Stop()
}

// IsRunning 检查监控器是否运行
func (m *Monitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetStartTime 获取启动时间
func (m *Monitor) GetStartTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startTime
}

// GetMetrics 获取当前指标
func (m *Monitor) GetMetrics() *Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回指标的副本
	metrics := &Metrics{
		Containers:    make([]ContainerMetric, len(m.metrics.Containers)),
		SystemMemory:  m.metrics.SystemMemory,
		EBPFMapsCount: m.metrics.EBPFMapsCount,
		LastUpdate:    m.metrics.LastUpdate,
	}
	copy(metrics.Containers, m.metrics.Containers)

	return metrics
}

// loadPrograms 加载 eBPF 程序
func (m *Monitor) loadPrograms() error {
	// 加载容器跟踪程序
	containerSpec, err := ebpf.LoadCollectionSpec("pkg/ebpf/container_trace.o")
	if err != nil {
		// 如果文件不存在，创建空的 spec (开发阶段)
		containerSpec = &ebpf.CollectionSpec{
			Maps: map[string]*ebpf.MapSpec{
				"container_map":       createMapSpec(ebpf.LRUHash, 8, int(unsafe.Sizeof(ContainerInfo{})), 1000),
				"pid_to_cgroup_map":   createMapSpec(ebpf.LRUHash, 4, 8, 10000),
				"events":              createMapSpec(ebpf.RingBuf, 0, 0, 256*1024),
				"stats_map":           createMapSpec(ebpf.Array, 4, 8, 10),
			},
			Programs: map[string]*ebpf.ProgramSpec{},
		}
	}

	// 加载网络监控程序
	networkSpec, err := ebpf.LoadCollectionSpec("pkg/ebpf/network_monitor.o")
	if err != nil {
		// 如果文件不存在，创建空的 spec (开发阶段)
		networkSpec = &ebpf.CollectionSpec{
			Maps: map[string]*ebpf.MapSpec{
				"flow_stats_map":    createMapSpec(ebpf.LRUHash, int(unsafe.Sizeof(FlowKey{})), int(unsafe.Sizeof(FlowStats{})), 10240),
				"latency_map":       createMapSpec(ebpf.LRUHash, int(unsafe.Sizeof(FlowKey{})), 8, 10240),
				"tcp_state_map":     createMapSpec(ebpf.LRUHash, int(unsafe.Sizeof(FlowKey{})), 4, 10240),
				"network_events":    createMapSpec(ebpf.RingBuf, 0, 0, 512*1024),
				"network_stats_map": createMapSpec(ebpf.Array, 4, 8, 20),
			},
			Programs: map[string]*ebpf.ProgramSpec{},
		}
	}

	// 合并 specs
	m.spec = &ebpf.CollectionSpec{
		Maps:     make(map[string]*ebpf.MapSpec),
		Programs: make(map[string]*ebpf.ProgramSpec),
	}

	// 复制容器跟踪 maps
	for name, mapSpec := range containerSpec.Maps {
		m.spec.Maps[name] = mapSpec
	}

	// 复制网络监控 maps
	for name, mapSpec := range networkSpec.Maps {
		m.spec.Maps[name] = mapSpec
	}

	// 复制程序
	for name, progSpec := range containerSpec.Programs {
		m.spec.Programs[name] = progSpec
	}
	for name, progSpec := range networkSpec.Programs {
		m.spec.Programs[name] = progSpec
	}

	// 加载 collection
	coll, err := ebpf.NewCollection(m.spec)
	if err != nil {
		return fmt.Errorf("创建 eBPF collection 失败: %w", err)
	}

	m.coll = coll
	return nil
}

// createMapSpec 创建 eBPF map 规格
func createMapSpec(mapType ebpf.MapType, keySize, valueSize, maxEntries int) *ebpf.MapSpec {
	return &ebpf.MapSpec{
		Type:       mapType,
		KeySize:    uint32(keySize),
		ValueSize:  uint32(valueSize),
		MaxEntries: uint32(maxEntries),
	}
}

// attachPrograms 附加 eBPF 程序到内核
func (m *Monitor) attachPrograms() error {
	// 附加容器跟踪程序
	if err := m.attachContainerTracing(); err != nil {
		return fmt.Errorf("附加容器跟踪程序失败: %w", err)
	}

	// 附加网络监控程序
	if err := m.attachNetworkMonitoring(); err != nil {
		return fmt.Errorf("附加网络监控程序失败: %w", err)
	}

	return nil
}

// attachContainerTracing 附加容器跟踪程序
func (m *Monitor) attachContainerTracing() error {
	// 附加 sys_enter_clone tracepoint
	if prog := m.coll.Programs["trace_container_start"]; prog != nil {
		l, err := link.Tracepoint(link.TracepointOptions{
			Group:   "syscalls",
			Name:    "sys_enter_clone",
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("附加 clone tracepoint 失败: %w", err)
		}
		m.links = append(m.links, l)
	}

	// 附加 sys_enter_exit tracepoint
	if prog := m.coll.Programs["trace_container_stop"]; prog != nil {
		l, err := link.Tracepoint(link.TracepointOptions{
			Group:   "syscalls",
			Name:    "sys_enter_exit",
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("附加 exit tracepoint 失败: %w", err)
		}
		m.links = append(m.links, l)
	}

	// 附加 cgroup_attach_task kprobe
	if prog := m.coll.Programs["kprobe_cgroup_attach"]; prog != nil {
		l, err := link.Kprobe(link.KprobeOptions{
			Symbol:  "cgroup_attach_task",
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("附加 cgroup kprobe 失败: %w", err)
		}
		m.links = append(m.links, l)
	}

	// 附加 sched_process_exec tracepoint
	if prog := m.coll.Programs["trace_process_exec"]; prog != nil {
		l, err := link.Tracepoint(link.TracepointOptions{
			Group:   "sched",
			Name:    "sched_process_exec",
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("附加 exec tracepoint 失败: %w", err)
		}
		m.links = append(m.links, l)
	}

	return nil
}

// attachNetworkMonitoring 附加网络监控程序
func (m *Monitor) attachNetworkMonitoring() error {
	// 附加 TCP 重传 kprobe
	if prog := m.coll.Programs["kprobe_tcp_retransmit"]; prog != nil {
		l, err := link.Kprobe(link.KprobeOptions{
			Symbol:  "tcp_retransmit_skb",
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("附加 TCP 重传 kprobe 失败: %w", err)
		}
		m.links = append(m.links, l)
	}

	// 附加 TCP probe tracepoint
	if prog := m.coll.Programs["trace_tcp_probe"]; prog != nil {
		l, err := link.Tracepoint(link.TracepointOptions{
			Group:   "tcp",
			Name:    "tcp_probe",
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("附加 TCP probe tracepoint 失败: %w", err)
		}
		m.links = append(m.links, l)
	}

	// TC 程序需要手动附加到网络接口
	// 这里暂时跳过，因为需要指定具体的网络接口

	return nil
}

// collectData 收集监控数据
func (m *Monitor) collectData() {
	ticker := time.NewTicker(m.config.Display.RefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !m.IsRunning() {
				return
			}
			m.updateMetrics()
		}
	}
}

// updateMetrics 更新指标数据
func (m *Monitor) updateMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics.LastUpdate = time.Now()
	m.metrics.EBPFMapsCount = len(m.coll.Maps)

	// 从 eBPF maps 读取容器数据
	containers, err := m.readContainerMetrics()
	if err != nil {
		// 如果读取失败，使用模拟数据 (开发阶段)
		if len(m.metrics.Containers) == 0 {
			m.metrics.Containers = m.generateMockContainers()
		}
		return
	}

	m.metrics.Containers = containers
}

// readContainerMetrics 从 eBPF maps 读取容器指标
func (m *Monitor) readContainerMetrics() ([]ContainerMetric, error) {
	containerMap := m.coll.Maps["container_map"]
	if containerMap == nil {
		return nil, fmt.Errorf("container_map 不存在")
	}

	flowStatsMap := m.coll.Maps["flow_stats_map"]
	if flowStatsMap == nil {
		return nil, fmt.Errorf("flow_stats_map 不存在")
	}

	var containers []ContainerMetric
	var key uint64
	var containerInfo ContainerInfo

	// 遍历容器映射表
	iter := containerMap.Iterate()
	for iter.Next(&key, &containerInfo) {
		container := ContainerMetric{
			ID:            fmt.Sprintf("%x", containerInfo.CgroupID),
			Name:          string(containerInfo.Comm[:]),
			PID:           containerInfo.PID,
			CPUPercent:    float64(containerInfo.CPUUsage) / 10.0, // 千分比转百分比
			MemoryUsage:   containerInfo.MemoryUsage,
			MemoryPercent: calculateMemoryPercent(containerInfo.MemoryUsage),
			Status:        containerStatusToString(containerInfo.Status),
			StartTime:     time.Unix(0, int64(containerInfo.StartTime)),
		}

		// 计算网络指标
		networkMetrics := m.calculateNetworkMetrics(containerInfo.CgroupID, flowStatsMap)
		container.NetworkLatency = networkMetrics.AvgLatency
		container.TCPRetransmits = networkMetrics.TCPRetransmits

		containers = append(containers, container)
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("遍历容器映射表失败: %w", err)
	}

	return containers, nil
}

// NetworkMetrics 网络指标
type NetworkMetrics struct {
	AvgLatency     float64
	TCPRetransmits uint32
	TotalPackets   uint64
	TotalBytes     uint64
}

// calculateNetworkMetrics 计算网络指标
func (m *Monitor) calculateNetworkMetrics(cgroupID uint64, flowStatsMap *ebpf.Map) NetworkMetrics {
	var metrics NetworkMetrics
	var key FlowKey
	var stats FlowStats

	// 遍历流量统计映射表
	iter := flowStatsMap.Iterate()
	for iter.Next(&key, &stats) {
		if key.CgroupID == cgroupID {
			metrics.TotalPackets += stats.Packets
			metrics.TotalBytes += stats.Bytes
			metrics.TCPRetransmits += stats.TCPRetransmits

			// 计算平均延迟
			if stats.LatencyCount > 0 {
				latencyMs := float64(stats.LatencySum) / float64(stats.LatencyCount) / 1000000.0 // 纳秒转毫秒
				if metrics.AvgLatency == 0 {
					metrics.AvgLatency = latencyMs
				} else {
					metrics.AvgLatency = (metrics.AvgLatency + latencyMs) / 2.0
				}
			}
		}
	}

	return metrics
}

// generateMockContainers 生成模拟容器数据 (开发阶段)
func (m *Monitor) generateMockContainers() []ContainerMetric {
	return []ContainerMetric{
		{
			ID:             "container_001",
			Name:           "web-server",
			PID:            12345,
			CPUPercent:     32.1,
			MemoryPercent:  45.6,
			MemoryUsage:    512 * 1024 * 1024, // 512MB
			NetworkLatency: 8.5,
			TCPRetransmits: 0,
			Status:         "running",
			StartTime:      time.Now().Add(-2 * time.Hour),
		},
		{
			ID:             "container_002",
			Name:           "db-primary",
			PID:            12346,
			CPUPercent:     78.9,
			MemoryPercent:  62.3,
			MemoryUsage:    1024 * 1024 * 1024, // 1GB
			NetworkLatency: 12.3,
			TCPRetransmits: 2,
			Status:         "running",
			StartTime:      time.Now().Add(-4 * time.Hour),
		},
	}
}

// calculateMemoryPercent 计算内存使用百分比
func calculateMemoryPercent(memoryUsage uint64) float64 {
	// 简化计算，假设系统总内存为 8GB
	totalMemory := uint64(8 * 1024 * 1024 * 1024)
	return float64(memoryUsage) / float64(totalMemory) * 100.0
}

// containerStatusToString 将容器状态转换为字符串
func containerStatusToString(status uint32) string {
	switch status {
	case 0: // CONTAINER_STATUS_UNKNOWN
		return "unknown"
	case 1: // CONTAINER_STATUS_CREATED
		return "created"
	case 2: // CONTAINER_STATUS_RUNNING
		return "running"
	case 3: // CONTAINER_STATUS_PAUSED
		return "paused"
	case 4: // CONTAINER_STATUS_STOPPED
		return "stopped"
	case 5: // CONTAINER_STATUS_EXITED
		return "exited"
	default:
		return "unknown"
	}
}

// cleanup 清理资源
func (m *Monitor) cleanup() {
	// 关闭所有链接
	for _, l := range m.links {
		if l != nil {
			l.Close()
		}
	}
	m.links = nil

	// 关闭 collection
	if m.coll != nil {
		m.coll.Close()
		m.coll = nil
	}
}

// GetProcessManager 获取进程管理器
func (m *Monitor) GetProcessManager() *ProcessManager {
	return m.processManager
}

// KillContainerProcess 取消容器进程
func (m *Monitor) KillContainerProcess(containerIndex int) error {
	return m.processManager.KillContainerProcess(containerIndex)
}

// KillContainerProcessWithOptions 使用选项取消容器进程
func (m *Monitor) KillContainerProcessWithOptions(containerIndex int, options KillProcessOptions) error {
	return m.processManager.KillContainerWithOptions(containerIndex, options)
}
