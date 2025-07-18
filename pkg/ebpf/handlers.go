package ebpf

import (
	"fmt"
	"log"
	"time"
)

// ContainerStartHandler 容器启动事件处理器
type ContainerStartHandler struct {
	processor *DataProcessor
}

// HandleEvent 处理容器启动事件
func (h *ContainerStartHandler) HandleEvent(event *EventData) error {
	log.Printf("处理容器启动事件: cgroup_id=%d, pid=%d", event.CgroupID, event.PID)
	
	h.processor.aggregator.mu.Lock()
	defer h.processor.aggregator.mu.Unlock()
	
	// 创建或更新容器指标
	container, exists := h.processor.aggregator.containerMetrics[event.CgroupID]
	if !exists {
		container = &AggregatedContainerMetrics{
			CgroupID:      event.CgroupID,
			ContainerID:   fmt.Sprintf("%x", event.CgroupID),
			PID:           event.PID,
			CPUSamples:    make([]float64, 0),
			MemorySamples: make([]uint64, 0),
			Status:        "starting",
			StartTime:     time.Unix(0, int64(event.Timestamp)),
			LastUpdate:    time.Now(),
		}
		h.processor.aggregator.containerMetrics[event.CgroupID] = container
	}
	
	container.Status = "running"
	container.LastUpdate = time.Now()
	
	return nil
}

// ContainerStopHandler 容器停止事件处理器
type ContainerStopHandler struct {
	processor *DataProcessor
}

// HandleEvent 处理容器停止事件
func (h *ContainerStopHandler) HandleEvent(event *EventData) error {
	log.Printf("处理容器停止事件: cgroup_id=%d, pid=%d", event.CgroupID, event.PID)
	
	h.processor.aggregator.mu.Lock()
	defer h.processor.aggregator.mu.Unlock()
	
	// 更新容器状态
	if container, exists := h.processor.aggregator.containerMetrics[event.CgroupID]; exists {
		container.Status = "stopped"
		container.LastUpdate = time.Now()
	}
	
	return nil
}

// NetworkPacketHandler 网络包事件处理器
type NetworkPacketHandler struct {
	processor *DataProcessor
}

// HandleEvent 处理网络包事件
func (h *NetworkPacketHandler) HandleEvent(event *EventData) error {
	h.processor.aggregator.mu.Lock()
	defer h.processor.aggregator.mu.Unlock()
	
	// 创建或更新网络指标
	network, exists := h.processor.aggregator.networkMetrics[event.CgroupID]
	if !exists {
		network = &AggregatedNetworkMetrics{
			CgroupID:       event.CgroupID,
			LatencySamples: make([]float64, 0),
			LastUpdate:     time.Now(),
		}
		h.processor.aggregator.networkMetrics[event.CgroupID] = network
	}
	
	// 更新网络统计
	network.LastUpdate = time.Now()
	
	// 这里可以解析 event.Data 中的网络统计信息
	// 由于数据结构复杂，暂时简化处理
	
	return nil
}

// CPUSampleHandler CPU 采样事件处理器
type CPUSampleHandler struct {
	processor *DataProcessor
}

// HandleEvent 处理 CPU 采样事件
func (h *CPUSampleHandler) HandleEvent(event *EventData) error {
	h.processor.aggregator.mu.Lock()
	defer h.processor.aggregator.mu.Unlock()
	
	// 查找容器指标
	container, exists := h.processor.aggregator.containerMetrics[event.CgroupID]
	if !exists {
		return fmt.Errorf("容器不存在: cgroup_id=%d", event.CgroupID)
	}
	
	// 从事件数据中提取 CPU 使用率
	// 这里需要解析 event.Data，暂时使用模拟数据
	cpuUsage := 50.0 // 模拟 CPU 使用率
	
	// 添加 CPU 样本
	container.CPUSamples = append(container.CPUSamples, cpuUsage)
	
	// 保持样本数量在合理范围内
	if len(container.CPUSamples) > 100 {
		container.CPUSamples = container.CPUSamples[1:]
	}
	
	// 计算统计值
	h.calculateCPUStats(container)
	container.LastUpdate = time.Now()
	
	return nil
}

// calculateCPUStats 计算 CPU 统计值
func (h *CPUSampleHandler) calculateCPUStats(container *AggregatedContainerMetrics) {
	if len(container.CPUSamples) == 0 {
		return
	}
	
	sum := 0.0
	min := container.CPUSamples[0]
	max := container.CPUSamples[0]
	
	for _, sample := range container.CPUSamples {
		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}
	
	container.CPUAvg = sum / float64(len(container.CPUSamples))
	container.CPUMin = min
	container.CPUMax = max
}

// MemorySampleHandler 内存采样事件处理器
type MemorySampleHandler struct {
	processor *DataProcessor
}

// HandleEvent 处理内存采样事件
func (h *MemorySampleHandler) HandleEvent(event *EventData) error {
	h.processor.aggregator.mu.Lock()
	defer h.processor.aggregator.mu.Unlock()
	
	// 查找容器指标
	container, exists := h.processor.aggregator.containerMetrics[event.CgroupID]
	if !exists {
		return fmt.Errorf("容器不存在: cgroup_id=%d", event.CgroupID)
	}
	
	// 从事件数据中提取内存使用量
	// 这里需要解析 event.Data，暂时使用模拟数据
	memoryUsage := uint64(512 * 1024 * 1024) // 模拟 512MB 内存使用
	
	// 添加内存样本
	container.MemorySamples = append(container.MemorySamples, memoryUsage)
	
	// 保持样本数量在合理范围内
	if len(container.MemorySamples) > 100 {
		container.MemorySamples = container.MemorySamples[1:]
	}
	
	// 计算统计值
	h.calculateMemoryStats(container)
	container.LastUpdate = time.Now()
	
	return nil
}

// calculateMemoryStats 计算内存统计值
func (h *MemorySampleHandler) calculateMemoryStats(container *AggregatedContainerMetrics) {
	if len(container.MemorySamples) == 0 {
		return
	}
	
	sum := uint64(0)
	min := container.MemorySamples[0]
	max := container.MemorySamples[0]
	
	for _, sample := range container.MemorySamples {
		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}
	
	container.MemoryAvg = sum / uint64(len(container.MemorySamples))
	container.MemoryMin = min
	container.MemoryMax = max
}

// AlertHandler 告警处理器
type AlertHandler struct {
	processor *DataProcessor
	thresholds *AlertThresholds
}

// AlertThresholds 告警阈值
type AlertThresholds struct {
	CPUPercent     float64
	MemoryPercent  float64
	NetworkLatency float64
}

// NewAlertHandler 创建告警处理器
func NewAlertHandler(processor *DataProcessor, thresholds *AlertThresholds) *AlertHandler {
	return &AlertHandler{
		processor:  processor,
		thresholds: thresholds,
	}
}

// CheckAlerts 检查告警
func (h *AlertHandler) CheckAlerts() []Alert {
	h.processor.aggregator.mu.RLock()
	defer h.processor.aggregator.mu.RUnlock()
	
	var alerts []Alert
	
	// 检查容器告警
	for cgroupID, container := range h.processor.aggregator.containerMetrics {
		// CPU 告警
		if container.CPUAvg >= h.thresholds.CPUPercent {
			alerts = append(alerts, Alert{
				Type:        "cpu_high",
				CgroupID:    cgroupID,
				ContainerID: container.ContainerID,
				Message:     fmt.Sprintf("CPU 使用率过高: %.1f%%", container.CPUAvg),
				Severity:    "warning",
				Timestamp:   time.Now(),
			})
		}
		
		// 内存告警
		memoryPercent := float64(container.MemoryAvg) / (8 * 1024 * 1024 * 1024) * 100 // 假设 8GB 总内存
		if memoryPercent >= h.thresholds.MemoryPercent {
			alerts = append(alerts, Alert{
				Type:        "memory_high",
				CgroupID:    cgroupID,
				ContainerID: container.ContainerID,
				Message:     fmt.Sprintf("内存使用率过高: %.1f%%", memoryPercent),
				Severity:    "warning",
				Timestamp:   time.Now(),
			})
		}
	}
	
	// 检查网络告警
	for cgroupID, network := range h.processor.aggregator.networkMetrics {
		if network.LatencyAvg >= h.thresholds.NetworkLatency {
			alerts = append(alerts, Alert{
				Type:        "network_latency_high",
				CgroupID:    cgroupID,
				Message:     fmt.Sprintf("网络延迟过高: %.1fms", network.LatencyAvg),
				Severity:    "warning",
				Timestamp:   time.Now(),
			})
		}
	}
	
	return alerts
}

// Alert 告警信息
type Alert struct {
	Type        string    `json:"type"`
	CgroupID    uint64    `json:"cgroup_id"`
	ContainerID string    `json:"container_id"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"`
	Timestamp   time.Time `json:"timestamp"`
}
