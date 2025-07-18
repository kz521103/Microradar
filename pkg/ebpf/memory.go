package ebpf

import (
	"runtime"
	"sync"
	"time"
)

// MemoryManager 内存管理器
type MemoryManager struct {
	maxMemory     uint64
	currentMemory uint64
	pools         map[string]*ObjectPool
	mu            sync.RWMutex
	
	// 内存监控
	memoryStats   *MemoryStats
	gcTicker      *time.Ticker
	gcInterval    time.Duration
}

// MemoryStats 内存统计
type MemoryStats struct {
	AllocatedBytes   uint64    `json:"allocated_bytes"`
	MaxAllocated     uint64    `json:"max_allocated"`
	GCCount          uint64    `json:"gc_count"`
	LastGC           time.Time `json:"last_gc"`
	PoolHits         uint64    `json:"pool_hits"`
	PoolMisses       uint64    `json:"pool_misses"`
	ObjectsInPool    int       `json:"objects_in_pool"`
	ObjectsAllocated int       `json:"objects_allocated"`
}

// ObjectPool 对象池
type ObjectPool struct {
	name     string
	objects  chan interface{}
	factory  func() interface{}
	reset    func(interface{})
	maxSize  int
	created  int64
	hits     int64
	misses   int64
	mu       sync.RWMutex
}

// NewMemoryManager 创建内存管理器
func NewMemoryManager(maxMemory uint64) *MemoryManager {
	mm := &MemoryManager{
		maxMemory:   maxMemory,
		pools:       make(map[string]*ObjectPool),
		memoryStats: &MemoryStats{},
		gcInterval:  30 * time.Second,
	}
	
	// 启动内存监控
	mm.startMemoryMonitoring()
	
	// 创建常用对象池
	mm.createDefaultPools()
	
	return mm
}

// createDefaultPools 创建默认对象池
func (mm *MemoryManager) createDefaultPools() {
	// 容器指标对象池
	mm.CreatePool("container_metrics", 100, 
		func() interface{} {
			return &ContainerMetric{}
		},
		func(obj interface{}) {
			if metric, ok := obj.(*ContainerMetric); ok {
				*metric = ContainerMetric{} // 重置对象
			}
		})
	
	// 网络流量键对象池
	mm.CreatePool("flow_keys", 1000,
		func() interface{} {
			return &FlowKey{}
		},
		func(obj interface{}) {
			if key, ok := obj.(*FlowKey); ok {
				*key = FlowKey{} // 重置对象
			}
		})
	
	// 网络流量统计对象池
	mm.CreatePool("flow_stats", 1000,
		func() interface{} {
			return &FlowStats{}
		},
		func(obj interface{}) {
			if stats, ok := obj.(*FlowStats); ok {
				*stats = FlowStats{} // 重置对象
			}
		})
	
	// 事件数据对象池
	mm.CreatePool("event_data", 500,
		func() interface{} {
			return &EventData{}
		},
		func(obj interface{}) {
			if event, ok := obj.(*EventData); ok {
				*event = EventData{} // 重置对象
			}
		})
	
	// 字节切片池 (用于缓冲区)
	mm.CreatePool("byte_buffers", 200,
		func() interface{} {
			return make([]byte, 4096) // 4KB 缓冲区
		},
		func(obj interface{}) {
			if buf, ok := obj.([]byte); ok {
				// 清零缓冲区
				for i := range buf {
					buf[i] = 0
				}
			}
		})
}

// CreatePool 创建对象池
func (mm *MemoryManager) CreatePool(name string, maxSize int, factory func() interface{}, reset func(interface{})) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	pool := &ObjectPool{
		name:    name,
		objects: make(chan interface{}, maxSize),
		factory: factory,
		reset:   reset,
		maxSize: maxSize,
	}
	
	mm.pools[name] = pool
}

// GetFromPool 从对象池获取对象
func (mm *MemoryManager) GetFromPool(poolName string) interface{} {
	mm.mu.RLock()
	pool, exists := mm.pools[poolName]
	mm.mu.RUnlock()
	
	if !exists {
		return nil
	}
	
	return pool.Get()
}

// PutToPool 将对象放回对象池
func (mm *MemoryManager) PutToPool(poolName string, obj interface{}) {
	mm.mu.RLock()
	pool, exists := mm.pools[poolName]
	mm.mu.RUnlock()
	
	if !exists {
		return
	}
	
	pool.Put(obj)
}

// Get 从对象池获取对象
func (p *ObjectPool) Get() interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	select {
	case obj := <-p.objects:
		p.hits++
		return obj
	default:
		p.misses++
		p.created++
		return p.factory()
	}
}

// Put 将对象放回对象池
func (p *ObjectPool) Put(obj interface{}) {
	if obj == nil {
		return
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// 重置对象
	if p.reset != nil {
		p.reset(obj)
	}
	
	// 尝试放回池中
	select {
	case p.objects <- obj:
		// 成功放回
	default:
		// 池已满，丢弃对象
	}
}

// Stats 获取对象池统计
func (p *ObjectPool) Stats() (created, hits, misses int64, poolSize int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return p.created, p.hits, p.misses, len(p.objects)
}

// startMemoryMonitoring 启动内存监控
func (mm *MemoryManager) startMemoryMonitoring() {
	mm.gcTicker = time.NewTicker(mm.gcInterval)
	
	go func() {
		for range mm.gcTicker.C {
			mm.performGC()
			mm.updateMemoryStats()
		}
	}()
}

// performGC 执行垃圾回收
func (mm *MemoryManager) performGC() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// 如果内存使用超过限制，强制 GC
	if m.Alloc > mm.maxMemory {
		runtime.GC()
		mm.memoryStats.GCCount++
		mm.memoryStats.LastGC = time.Now()
	}
}

// updateMemoryStats 更新内存统计
func (mm *MemoryManager) updateMemoryStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	mm.currentMemory = m.Alloc
	mm.memoryStats.AllocatedBytes = m.Alloc
	
	if m.Alloc > mm.memoryStats.MaxAllocated {
		mm.memoryStats.MaxAllocated = m.Alloc
	}
	
	// 统计对象池信息
	totalPoolHits := uint64(0)
	totalPoolMisses := uint64(0)
	totalObjectsInPool := 0
	totalObjectsAllocated := 0
	
	for _, pool := range mm.pools {
		created, hits, misses, poolSize := pool.Stats()
		totalPoolHits += uint64(hits)
		totalPoolMisses += uint64(misses)
		totalObjectsInPool += poolSize
		totalObjectsAllocated += int(created)
	}
	
	mm.memoryStats.PoolHits = totalPoolHits
	mm.memoryStats.PoolMisses = totalPoolMisses
	mm.memoryStats.ObjectsInPool = totalObjectsInPool
	mm.memoryStats.ObjectsAllocated = totalObjectsAllocated
}

// GetMemoryStats 获取内存统计
func (mm *MemoryManager) GetMemoryStats() *MemoryStats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	// 返回统计的副本
	stats := *mm.memoryStats
	return &stats
}

// GetCurrentMemory 获取当前内存使用量
func (mm *MemoryManager) GetCurrentMemory() uint64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.currentMemory
}

// IsMemoryLimitExceeded 检查是否超过内存限制
func (mm *MemoryManager) IsMemoryLimitExceeded() bool {
	return mm.GetCurrentMemory() > mm.maxMemory
}

// ForceGC 强制垃圾回收
func (mm *MemoryManager) ForceGC() {
	runtime.GC()
	mm.memoryStats.GCCount++
	mm.memoryStats.LastGC = time.Now()
}

// Close 关闭内存管理器
func (mm *MemoryManager) Close() {
	if mm.gcTicker != nil {
		mm.gcTicker.Stop()
	}
}

// MemoryOptimizer 内存优化器
type MemoryOptimizer struct {
	manager    *MemoryManager
	thresholds *MemoryThresholds
}

// MemoryThresholds 内存阈值
type MemoryThresholds struct {
	Warning  uint64 // 警告阈值
	Critical uint64 // 严重阈值
	Maximum  uint64 // 最大阈值
}

// NewMemoryOptimizer 创建内存优化器
func NewMemoryOptimizer(maxMemory uint64) *MemoryOptimizer {
	return &MemoryOptimizer{
		manager: NewMemoryManager(maxMemory),
		thresholds: &MemoryThresholds{
			Warning:  maxMemory * 70 / 100, // 70%
			Critical: maxMemory * 85 / 100, // 85%
			Maximum:  maxMemory,            // 100%
		},
	}
}

// OptimizeMemory 优化内存使用
func (mo *MemoryOptimizer) OptimizeMemory() {
	currentMemory := mo.manager.GetCurrentMemory()
	
	if currentMemory > mo.thresholds.Critical {
		// 严重情况：强制 GC 并清理对象池
		mo.manager.ForceGC()
		mo.clearObjectPools()
	} else if currentMemory > mo.thresholds.Warning {
		// 警告情况：执行 GC
		mo.manager.ForceGC()
	}
}

// clearObjectPools 清理对象池
func (mo *MemoryOptimizer) clearObjectPools() {
	mo.manager.mu.Lock()
	defer mo.manager.mu.Unlock()
	
	// 清理一半的对象池对象
	for _, pool := range mo.manager.pools {
		poolSize := len(pool.objects)
		clearCount := poolSize / 2
		
		for i := 0; i < clearCount; i++ {
			select {
			case <-pool.objects:
				// 移除对象
			default:
				break
			}
		}
	}
}

// GetManager 获取内存管理器
func (mo *MemoryOptimizer) GetManager() *MemoryManager {
	return mo.manager
}
