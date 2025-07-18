package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/micro-radar/pkg/config"
	"github.com/micro-radar/pkg/ebpf"
)

// DaemonServer 守护进程服务器
type DaemonServer struct {
	monitor *ebpf.Monitor
	config  *config.Config
	server  *http.Server
}

// NewDaemonServer 创建新的守护进程服务器
func NewDaemonServer(monitor *ebpf.Monitor, cfg *config.Config) *DaemonServer {
	return &DaemonServer{
		monitor: monitor,
		config:  cfg,
	}
}

// Start 启动守护进程服务
func (d *DaemonServer) Start() error {
	mux := http.NewServeMux()
	
	// 健康检查端点
	mux.HandleFunc("/health", d.healthHandler)
	
	// 指标端点 (Prometheus格式)
	mux.HandleFunc("/metrics", d.metricsHandler)
	
	// 状态端点
	mux.HandleFunc("/status", d.statusHandler)

	d.server = &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("守护进程HTTP服务启动在端口 :8080")
	return d.server.ListenAndServe()
}

// Stop 停止守护进程服务
func (d *DaemonServer) Stop() error {
	if d.server == nil {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	return d.server.Shutdown(ctx)
}

// healthHandler 健康检查处理器
func (d *DaemonServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	status := "healthy"
	if !d.monitor.IsRunning() {
		status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	fmt.Fprintf(w, `{"status": "%s", "timestamp": "%s"}`, 
		status, time.Now().Format(time.RFC3339))
}

// metricsHandler Prometheus指标处理器
func (d *DaemonServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	
	metrics := d.monitor.GetMetrics()
	
	// 输出Prometheus格式的指标
	fmt.Fprintf(w, "# HELP microradar_containers_total 监控的容器总数\n")
	fmt.Fprintf(w, "# TYPE microradar_containers_total gauge\n")
	fmt.Fprintf(w, "microradar_containers_total %d\n", len(metrics.Containers))
	
	fmt.Fprintf(w, "# HELP microradar_memory_usage_bytes 内存使用量\n")
	fmt.Fprintf(w, "# TYPE microradar_memory_usage_bytes gauge\n")
	fmt.Fprintf(w, "microradar_memory_usage_bytes %d\n", metrics.SystemMemory)
	
	// 为每个容器输出指标
	for _, container := range metrics.Containers {
		labels := fmt.Sprintf(`{container_id="%s",container_name="%s"}`, 
			container.ID, container.Name)
		
		fmt.Fprintf(w, "microradar_container_cpu_percent%s %.2f\n", 
			labels, container.CPUPercent)
		fmt.Fprintf(w, "microradar_container_memory_percent%s %.2f\n", 
			labels, container.MemoryPercent)
		fmt.Fprintf(w, "microradar_container_network_latency_ms%s %.2f\n", 
			labels, container.NetworkLatency)
	}
}

// statusHandler 状态信息处理器
func (d *DaemonServer) statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	metrics := d.monitor.GetMetrics()
	uptime := time.Since(d.monitor.GetStartTime())
	
	fmt.Fprintf(w, `{
		"version": "%s",
		"uptime_seconds": %.0f,
		"containers_monitored": %d,
		"memory_usage_mb": %.2f,
		"ebpf_maps_count": %d,
		"last_update": "%s"
	}`, 
		Version,
		uptime.Seconds(),
		len(metrics.Containers),
		float64(metrics.SystemMemory)/1024/1024,
		metrics.EBPFMapsCount,
		metrics.LastUpdate.Format(time.RFC3339))
}
