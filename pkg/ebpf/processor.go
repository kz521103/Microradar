package ebpf

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/kz521103/Microradar/pkg/config"
)

// DataProcessor 数据处理引擎
type DataProcessor struct {
	config     *config.Config
	monitor    *Monitor
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	
	// 数据聚合
	aggregator *MetricsAggregator
	
	// 事件处理
	eventHandlers map[uint32]EventHandler
}

// EventHandler 事件处理器接口
type EventHandler interface {
	HandleEvent(event *EventData) error
}

// MetricsAggregator 指标聚合器
type MetricsAggregator struct {
	containerMetrics map[uint64]*AggregatedContainerMetrics
	networkMetrics   map[uint64]*AggregatedNetworkMetrics
	systemMetrics    *SystemMetrics
	mu               sync.RWMutex
	
	// 聚合窗口
	windowSize time.Duration
	lastReset  time.Time
}

// AggregatedContainerMetrics 聚合的容器指标
type AggregatedContainerMetrics struct {
	CgroupID       uint64    `json:"cgroup_id"`
	ContainerID    string    `json:"container_id"`
	Name           string    `json:"name"`
	PID            uint32    `json:"pid"`
	
	// CPU 指标
	CPUSamples     []float64 `json:"cpu_samples"`
	CPUAvg         float64   `json:"cpu_avg"`
	CPUMax         float64   `json:"cpu_max"`
	CPUMin         float64   `json:"cpu_min"`
	
	// 内存指标
	MemorySamples  []uint64  `json:"memory_samples"`
	MemoryAvg      uint64    `json:"memory_avg"`
	MemoryMax      uint64    `json:"memory_max"`
	MemoryMin      uint64    `json:"memory_min"`
	
	// 状态
	Status         string    `json:"status"`
	StartTime      time.Time `json:"start_time"`
	LastUpdate     time.Time `json:"last_update"`
}

// AggregatedNetworkMetrics 聚合的网络指标
type AggregatedNetworkMetrics struct {
	CgroupID         uint64    `json:"cgroup_id"`
	
	// 流量统计
	TotalPacketsIn   uint64    `json:"total_packets_in"`
	TotalPacketsOut  uint64    `json:"total_packets_out"`
	TotalBytesIn     uint64    `json:"total_bytes_in"`
	TotalBytesOut    uint64    `json:"total_bytes_out"`
	
	// 延迟统计
	LatencySamples   []float64 `json:"latency_samples"`
	LatencyAvg       float64   `json:"latency_avg"`
	LatencyMax       float64   `json:"latency_max"`
	LatencyMin       float64   `json:"latency_min"`
	
	// TCP 统计
	TCPRetransmits   uint32    `json:"tcp_retransmits"`
	TCPConnections   uint32    `json:"tcp_connections"`
	
	// 时间戳
	LastUpdate       time.Time `json:"last_update"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	TotalContainers    int       `json:"total_containers"`
	ActiveContainers   int       `json:"active_containers"`
	TotalMemoryUsage   uint64    `json:"total_memory_usage"`
	TotalCPUUsage      float64   `json:"total_cpu_usage"`
	NetworkThroughput  uint64    `json:"network_throughput"`
	EBPFMapsCount      int       `json:"ebpf_maps_count"`
	EventsProcessed    uint64    `json:"events_processed"`
	LastUpdate         time.Time `json:"last_update"`
}

// NewDataProcessor 创建新的数据处理引擎
func NewDataProcessor(cfg *config.Config, monitor *Monitor) *DataProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	
	processor := &DataProcessor{
		config:        cfg,
		monitor:       monitor,
		ctx:           ctx,
		cancel:        cancel,
		eventHandlers: make(map[uint32]EventHandler),
		aggregator: &MetricsAggregator{
			containerMetrics: make(map[uint64]*AggregatedContainerMetrics),
			networkMetrics:   make(map[uint64]*AggregatedNetworkMetrics),
			systemMetrics:    &SystemMetrics{},
			windowSize:       60 * time.Second, // 60秒聚合窗口
			lastReset:        time.Now(),
		},
	}
	
	// 注册事件处理器
	processor.registerEventHandlers()
	
	return processor
}

// Start 启动数据处理引擎
func (p *DataProcessor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		return fmt.Errorf("数据处理引擎已在运行")
	}
	
	// 启动事件处理协程
	go p.processEvents()
	
	// 启动指标聚合协程
	go p.aggregateMetrics()
	
	// 启动清理协程
	go p.cleanup()
	
	p.running = true
	log.Println("数据处理引擎已启动")
	
	return nil
}

// Stop 停止数据处理引擎
func (p *DataProcessor) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.running {
		return nil
	}
	
	p.cancel()
	p.running = false
	
	log.Println("数据处理引擎已停止")
	return nil
}

// registerEventHandlers 注册事件处理器
func (p *DataProcessor) registerEventHandlers() {
	p.eventHandlers[1] = &ContainerStartHandler{processor: p} // EVENT_CONTAINER_START
	p.eventHandlers[2] = &ContainerStopHandler{processor: p}  // EVENT_CONTAINER_STOP
	p.eventHandlers[3] = &NetworkPacketHandler{processor: p}  // EVENT_NETWORK_PACKET
	p.eventHandlers[4] = &CPUSampleHandler{processor: p}      // EVENT_CPU_SAMPLE
	p.eventHandlers[5] = &MemorySampleHandler{processor: p}   // EVENT_MEMORY_SAMPLE
}

// processEvents 处理 eBPF 事件
func (p *DataProcessor) processEvents() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.readAndProcessEvents()
		}
	}
}

// readAndProcessEvents 读取并处理事件
func (p *DataProcessor) readAndProcessEvents() {
	// 从容器事件环形缓冲区读取
	if eventsMap := p.monitor.coll.Maps["events"]; eventsMap != nil {
		p.readEventsFromRingBuf(eventsMap)
	}
	
	// 从网络事件环形缓冲区读取
	if networkEventsMap := p.monitor.coll.Maps["network_events"]; networkEventsMap != nil {
		p.readEventsFromRingBuf(networkEventsMap)
	}
}

// readEventsFromRingBuf 从环形缓冲区读取事件
func (p *DataProcessor) readEventsFromRingBuf(ringBuf *ebpf.Map) {
	// TODO: 实现环形缓冲区读取
	// 这需要使用 cilium/ebpf 的 ringbuf reader
	// 由于复杂性，这里暂时跳过实际实现
}

// aggregateMetrics 聚合指标
func (p *DataProcessor) aggregateMetrics() {
	ticker := time.NewTicker(p.config.Display.RefreshRate)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performAggregation()
		}
	}
}

// performAggregation 执行指标聚合
func (p *DataProcessor) performAggregation() {
	p.aggregator.mu.Lock()
	defer p.aggregator.mu.Unlock()
	
	// 检查是否需要重置聚合窗口
	if time.Since(p.aggregator.lastReset) >= p.aggregator.windowSize {
		p.resetAggregationWindow()
	}
	
	// 从 eBPF maps 读取最新数据并聚合
	p.aggregateContainerMetrics()
	p.aggregateNetworkMetrics()
	p.aggregateSystemMetrics()
}

// aggregateContainerMetrics 聚合容器指标
func (p *DataProcessor) aggregateContainerMetrics() {
	// TODO: 从 eBPF maps 读取容器数据并聚合
}

// aggregateNetworkMetrics 聚合网络指标
func (p *DataProcessor) aggregateNetworkMetrics() {
	// TODO: 从 eBPF maps 读取网络数据并聚合
}

// aggregateSystemMetrics 聚合系统指标
func (p *DataProcessor) aggregateSystemMetrics() {
	p.aggregator.systemMetrics.LastUpdate = time.Now()
	p.aggregator.systemMetrics.TotalContainers = len(p.aggregator.containerMetrics)
	
	// 计算活跃容器数
	activeCount := 0
	for _, container := range p.aggregator.containerMetrics {
		if container.Status == "running" {
			activeCount++
		}
	}
	p.aggregator.systemMetrics.ActiveContainers = activeCount
}

// resetAggregationWindow 重置聚合窗口
func (p *DataProcessor) resetAggregationWindow() {
	// 清理过期的指标数据
	for cgroupID, container := range p.aggregator.containerMetrics {
		if time.Since(container.LastUpdate) > p.aggregator.windowSize*2 {
			delete(p.aggregator.containerMetrics, cgroupID)
		}
	}
	
	for cgroupID, network := range p.aggregator.networkMetrics {
		if time.Since(network.LastUpdate) > p.aggregator.windowSize*2 {
			delete(p.aggregator.networkMetrics, cgroupID)
		}
	}
	
	p.aggregator.lastReset = time.Now()
}

// cleanup 清理协程
func (p *DataProcessor) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performCleanup()
		}
	}
}

// performCleanup 执行清理
func (p *DataProcessor) performCleanup() {
	p.aggregator.mu.Lock()
	defer p.aggregator.mu.Unlock()
	
	// 清理过期数据
	cutoff := time.Now().Add(-10 * time.Minute)
	
	for cgroupID, container := range p.aggregator.containerMetrics {
		if container.LastUpdate.Before(cutoff) {
			delete(p.aggregator.containerMetrics, cgroupID)
		}
	}
	
	for cgroupID, network := range p.aggregator.networkMetrics {
		if network.LastUpdate.Before(cutoff) {
			delete(p.aggregator.networkMetrics, cgroupID)
		}
	}
}

// GetAggregatedMetrics 获取聚合指标
func (p *DataProcessor) GetAggregatedMetrics() (*MetricsAggregator, error) {
	p.aggregator.mu.RLock()
	defer p.aggregator.mu.RUnlock()
	
	// 返回聚合器的副本
	return p.aggregator, nil
}
