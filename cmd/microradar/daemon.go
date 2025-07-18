package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/kz521103/Microradar/pkg/config"
	"github.com/kz521103/Microradar/pkg/ebpf"
)

// runDaemon 运行守护进程模式
func runDaemon(cfg *config.Config, monitor *ebpf.Monitor, sigChan chan os.Signal) {
	// 创建 HTTP 服务器
	mux := http.NewServeMux()
	
	// 健康检查端点
	mux.HandleFunc("/health", healthHandler)
	
	// 指标端点
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metricsHandler(w, r, monitor)
	})
	
	// 状态端点
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		statusHandler(w, r, monitor)
	})
	
	// API 端点
	mux.HandleFunc("/api/containers", func(w http.ResponseWriter, r *http.Request) {
		containersHandler(w, r, monitor)
	})
	
	// 进程管理端点
	mux.HandleFunc("/api/containers/kill", func(w http.ResponseWriter, r *http.Request) {
		killContainerHandler(w, r, monitor)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// 启动 HTTP 服务器
	go func() {
		log.Println("HTTP 服务器启动在 :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务器启动失败: %v", err)
		}
	}()

	// 等待退出信号
	<-sigChan
	
	// 优雅关闭
	log.Println("正在关闭服务器...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭失败: %v", err)
	}
}

// healthHandler 健康检查处理器
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   Version,
	}
	
	json.NewEncoder(w).Encode(response)
}

// metricsHandler Prometheus 指标处理器
func metricsHandler(w http.ResponseWriter, r *http.Request, monitor *ebpf.Monitor) {
	w.Header().Set("Content-Type", "text/plain")
	
	metrics := monitor.GetMetrics()
	if metrics == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "# HELP microradar_up MicroRadar service status\n")
		fmt.Fprintf(w, "# TYPE microradar_up gauge\n")
		fmt.Fprintf(w, "microradar_up 0\n")
		return
	}

	// 基本指标
	fmt.Fprintf(w, "# HELP microradar_up MicroRadar service status\n")
	fmt.Fprintf(w, "# TYPE microradar_up gauge\n")
	fmt.Fprintf(w, "microradar_up 1\n")
	
	fmt.Fprintf(w, "# HELP microradar_containers_total Total number of monitored containers\n")
	fmt.Fprintf(w, "# TYPE microradar_containers_total gauge\n")
	fmt.Fprintf(w, "microradar_containers_total %d\n", len(metrics.Containers))
	
	fmt.Fprintf(w, "# HELP microradar_ebpf_maps_count Number of eBPF maps\n")
	fmt.Fprintf(w, "# TYPE microradar_ebpf_maps_count gauge\n")
	fmt.Fprintf(w, "microradar_ebpf_maps_count %d\n", metrics.EBPFMapsCount)

	// 容器指标
	for _, container := range metrics.Containers {
		labels := fmt.Sprintf(`container_id="%s",container_name="%s"`, container.ID, container.Name)
		
		fmt.Fprintf(w, "# HELP microradar_container_cpu_percent Container CPU usage percentage\n")
		fmt.Fprintf(w, "# TYPE microradar_container_cpu_percent gauge\n")
		fmt.Fprintf(w, "microradar_container_cpu_percent{%s} %.2f\n", labels, container.CPUPercent)
		
		fmt.Fprintf(w, "# HELP microradar_container_memory_percent Container memory usage percentage\n")
		fmt.Fprintf(w, "# TYPE microradar_container_memory_percent gauge\n")
		fmt.Fprintf(w, "microradar_container_memory_percent{%s} %.2f\n", labels, container.MemoryPercent)
		
		fmt.Fprintf(w, "# HELP microradar_container_memory_bytes Container memory usage in bytes\n")
		fmt.Fprintf(w, "# TYPE microradar_container_memory_bytes gauge\n")
		fmt.Fprintf(w, "microradar_container_memory_bytes{%s} %d\n", labels, container.MemoryUsage)
		
		fmt.Fprintf(w, "# HELP microradar_container_network_latency_ms Container network latency in milliseconds\n")
		fmt.Fprintf(w, "# TYPE microradar_container_network_latency_ms gauge\n")
		fmt.Fprintf(w, "microradar_container_network_latency_ms{%s} %.2f\n", labels, container.NetworkLatency)
		
		fmt.Fprintf(w, "# HELP microradar_container_tcp_retransmits Container TCP retransmissions\n")
		fmt.Fprintf(w, "# TYPE microradar_container_tcp_retransmits counter\n")
		fmt.Fprintf(w, "microradar_container_tcp_retransmits{%s} %d\n", labels, container.TCPRetransmits)
	}
}

// statusHandler 状态处理器
func statusHandler(w http.ResponseWriter, r *http.Request, monitor *ebpf.Monitor) {
	w.Header().Set("Content-Type", "application/json")
	
	metrics := monitor.GetMetrics()
	uptime := time.Since(monitor.GetStartTime())
	
	status := map[string]interface{}{
		"version":             Version,
		"commit":              Commit,
		"build_time":          BuildTime,
		"uptime_seconds":      int(uptime.Seconds()),
		"containers_monitored": 0,
		"ebpf_maps_count":     0,
		"last_update":         time.Now().UTC().Format(time.RFC3339),
	}
	
	if metrics != nil {
		status["containers_monitored"] = len(metrics.Containers)
		status["ebpf_maps_count"] = metrics.EBPFMapsCount
		status["last_update"] = metrics.LastUpdate.UTC().Format(time.RFC3339)
	}
	
	json.NewEncoder(w).Encode(status)
}

// containersHandler 容器列表处理器
func containersHandler(w http.ResponseWriter, r *http.Request, monitor *ebpf.Monitor) {
	w.Header().Set("Content-Type", "application/json")
	
	metrics := monitor.GetMetrics()
	if metrics == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "监控服务不可用"})
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"containers": metrics.Containers,
		"total":      len(metrics.Containers),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// killContainerHandler 取消容器处理器
func killContainerHandler(w http.ResponseWriter, r *http.Request, monitor *ebpf.Monitor) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "仅支持 POST 方法"})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	
	var request struct {
		ContainerIndex int  `json:"container_index"`
		Force          bool `json:"force"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "无效的请求格式"})
		return
	}
	
	// 执行取消操作
	var err error
	if request.Force {
		options := ebpf.KillProcessOptions{
			Force:       true,
			GracePeriod: 0,
		}
		err = monitor.KillContainerProcessWithOptions(request.ContainerIndex, options)
	} else {
		err = monitor.KillContainerProcess(request.ContainerIndex)
	}
	
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "容器进程取消成功",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
