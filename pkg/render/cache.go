package render

import (
	"sync"
	"time"

	"github.com/kz521103/Microradar/pkg/ebpf"
)

// RenderCache 渲染缓存系统
type RenderCache struct {
	mu                sync.RWMutex
	lastMetrics       *ebpf.Metrics
	lastRenderTime    time.Time
	cachedContainers  []CachedContainer
	cachedNetwork     []CachedNetwork
	cachedSystem      *CachedSystem
	cacheValid        bool
	cacheDuration     time.Duration
}

// CachedContainer 缓存的容器信息
type CachedContainer struct {
	ID             string
	Name           string
	PID            uint32
	CPUPercent     float64
	MemoryPercent  float64
	MemoryUsage    uint64
	NetworkLatency float64
	TCPRetransmits uint32
	Status         string
	StartTime      time.Time
	
	// 渲染相关
	CPUColor       int
	MemoryColor    int
	LatencyColor   int
	StatusColor    int
	FormattedCPU   string
	FormattedMem   string
	FormattedLat   string
}

// CachedNetwork 缓存的网络信息
type CachedNetwork struct {
	ContainerName  string
	PacketsIn      uint64
	PacketsOut     uint64
	BytesIn        uint64
	BytesOut       uint64
	Latency        float64
	Retransmits    uint32
	
	// 格式化字符串
	FormattedBytesIn  string
	FormattedBytesOut string
	FormattedLatency  string
}

// CachedSystem 缓存的系统信息
type CachedSystem struct {
	TotalContainers   int
	RunningContainers int
	EBPFMapsCount     int
	SystemMemory      string
	LastUpdate        string
	
	// 分布统计
	CPUDistribution    map[int]int
	MemoryDistribution map[int]int
	AlertCounts        AlertCounts
}

// NewRenderCache 创建渲染缓存
func NewRenderCache(cacheDuration time.Duration) *RenderCache {
	return &RenderCache{
		cacheDuration: cacheDuration,
		cacheValid:    false,
	}
}

// GetCachedData 获取缓存数据
func (rc *RenderCache) GetCachedData(metrics *ebpf.Metrics) ([]CachedContainer, []CachedNetwork, *CachedSystem, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	
	// 检查缓存是否有效
	if !rc.cacheValid || time.Since(rc.lastRenderTime) > rc.cacheDuration {
		return nil, nil, nil, false
	}
	
	// 检查数据是否变化
	if !rc.metricsEqual(metrics, rc.lastMetrics) {
		return nil, nil, nil, false
	}
	
	return rc.cachedContainers, rc.cachedNetwork, rc.cachedSystem, true
}

// UpdateCache 更新缓存
func (rc *RenderCache) UpdateCache(metrics *ebpf.Metrics, containers []CachedContainer, 
	network []CachedNetwork, system *CachedSystem) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	
	rc.lastMetrics = rc.copyMetrics(metrics)
	rc.lastRenderTime = time.Now()
	rc.cachedContainers = containers
	rc.cachedNetwork = network
	rc.cachedSystem = system
	rc.cacheValid = true
}

// InvalidateCache 使缓存失效
func (rc *RenderCache) InvalidateCache() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	
	rc.cacheValid = false
}

// metricsEqual 比较两个指标是否相等
func (rc *RenderCache) metricsEqual(m1, m2 *ebpf.Metrics) bool {
	if m1 == nil || m2 == nil {
		return false
	}
	
	if len(m1.Containers) != len(m2.Containers) {
		return false
	}
	
	if m1.SystemMemory != m2.SystemMemory {
		return false
	}
	
	if m1.EBPFMapsCount != m2.EBPFMapsCount {
		return false
	}
	
	// 比较容器数据
	for i, c1 := range m1.Containers {
		if i >= len(m2.Containers) {
			return false
		}
		
		c2 := m2.Containers[i]
		if c1.ID != c2.ID || 
		   c1.CPUPercent != c2.CPUPercent ||
		   c1.MemoryPercent != c2.MemoryPercent ||
		   c1.NetworkLatency != c2.NetworkLatency ||
		   c1.TCPRetransmits != c2.TCPRetransmits ||
		   c1.Status != c2.Status {
			return false
		}
	}
	
	return true
}

// copyMetrics 复制指标数据
func (rc *RenderCache) copyMetrics(metrics *ebpf.Metrics) *ebpf.Metrics {
	if metrics == nil {
		return nil
	}
	
	copy := &ebpf.Metrics{
		SystemMemory:  metrics.SystemMemory,
		EBPFMapsCount: metrics.EBPFMapsCount,
		LastUpdate:    metrics.LastUpdate,
		Containers:    make([]ebpf.ContainerMetric, len(metrics.Containers)),
	}
	
	for i, container := range metrics.Containers {
		copy.Containers[i] = container
	}
	
	return copy
}

// FrameRateController 帧率控制器
type FrameRateController struct {
	targetFPS     int
	frameDuration time.Duration
	lastFrame     time.Time
	frameCount    int
	fpsStartTime  time.Time
	currentFPS    float64
	mu            sync.RWMutex
}

// NewFrameRateController 创建帧率控制器
func NewFrameRateController(targetFPS int) *FrameRateController {
	return &FrameRateController{
		targetFPS:     targetFPS,
		frameDuration: time.Duration(1000/targetFPS) * time.Millisecond,
		fpsStartTime:  time.Now(),
	}
}

// ShouldRender 检查是否应该渲染
func (frc *FrameRateController) ShouldRender() bool {
	frc.mu.Lock()
	defer frc.mu.Unlock()
	
	now := time.Now()
	
	// 检查是否到了下一帧的时间
	if now.Sub(frc.lastFrame) < frc.frameDuration {
		return false
	}
	
	frc.lastFrame = now
	frc.frameCount++
	
	// 每秒计算一次 FPS
	if now.Sub(frc.fpsStartTime) >= time.Second {
		frc.currentFPS = float64(frc.frameCount) / now.Sub(frc.fpsStartTime).Seconds()
		frc.frameCount = 0
		frc.fpsStartTime = now
	}
	
	return true
}

// GetCurrentFPS 获取当前 FPS
func (frc *FrameRateController) GetCurrentFPS() float64 {
	frc.mu.RLock()
	defer frc.mu.RUnlock()
	return frc.currentFPS
}

// SetTargetFPS 设置目标 FPS
func (frc *FrameRateController) SetTargetFPS(fps int) {
	frc.mu.Lock()
	defer frc.mu.Unlock()
	
	frc.targetFPS = fps
	frc.frameDuration = time.Duration(1000/fps) * time.Millisecond
}

// RenderOptimizer 渲染优化器
type RenderOptimizer struct {
	cache           *RenderCache
	frameController *FrameRateController
	dirtyRegions    []Region
	fullRedraw      bool
	mu              sync.RWMutex
}

// Region 脏区域
type Region struct {
	X, Y, Width, Height int
}

// NewRenderOptimizer 创建渲染优化器
func NewRenderOptimizer(targetFPS int, cacheDuration time.Duration) *RenderOptimizer {
	return &RenderOptimizer{
		cache:           NewRenderCache(cacheDuration),
		frameController: NewFrameRateController(targetFPS),
		dirtyRegions:    make([]Region, 0),
		fullRedraw:      true,
	}
}

// ShouldRender 检查是否应该渲染
func (ro *RenderOptimizer) ShouldRender() bool {
	return ro.frameController.ShouldRender()
}

// MarkDirty 标记脏区域
func (ro *RenderOptimizer) MarkDirty(x, y, width, height int) {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	
	ro.dirtyRegions = append(ro.dirtyRegions, Region{
		X: x, Y: y, Width: width, Height: height,
	})
}

// MarkFullRedraw 标记全屏重绘
func (ro *RenderOptimizer) MarkFullRedraw() {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	
	ro.fullRedraw = true
	ro.dirtyRegions = ro.dirtyRegions[:0]
}

// GetDirtyRegions 获取脏区域
func (ro *RenderOptimizer) GetDirtyRegions() ([]Region, bool) {
	ro.mu.Lock()
	defer ro.mu.Unlock()
	
	if ro.fullRedraw {
		ro.fullRedraw = false
		return nil, true
	}
	
	regions := make([]Region, len(ro.dirtyRegions))
	copy(regions, ro.dirtyRegions)
	ro.dirtyRegions = ro.dirtyRegions[:0]
	
	return regions, false
}

// GetCache 获取缓存
func (ro *RenderOptimizer) GetCache() *RenderCache {
	return ro.cache
}

// GetCurrentFPS 获取当前 FPS
func (ro *RenderOptimizer) GetCurrentFPS() float64 {
	return ro.frameController.GetCurrentFPS()
}
