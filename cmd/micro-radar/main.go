package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kz521103/Microradar/pkg/config"
	"github.com/kz521103/Microradar/pkg/ebpf"
	"github.com/kz521103/Microradar/pkg/render"
)

const (
	Version = "1.0.0"
	Banner  = `
 __  __ _                 ____           _            
|  \/  (_) ___ _ __ ___  |  _ \ __ _  __| | __ _ _ __ 
| |\/| | |/ __| '__/ _ \ | |_) / _' |/ _' |/ _' | '__|
| |  | | | (__| | | (_) ||  _ < (_| | (_| | (_| | |   
|_|  |_|_|\___|_|  \___/ |_| \_\__,_|\__,_|\__,_|_|   
                                                     
轻量级容器监控工具 v%s
`
)

var (
	configFile = flag.String("config", "config.yaml", "配置文件路径")
	initConfig = flag.Bool("init", false, "生成默认配置文件")
	version    = flag.Bool("version", false, "显示版本信息")
	daemon     = flag.Bool("daemon", false, "以守护进程模式运行")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("MicroRadar v%s\n", Version)
		fmt.Println("构建信息: Go 1.21.4, eBPF enabled")
		os.Exit(0)
	}

	if *initConfig {
		generateDefaultConfig()
		return
	}

	fmt.Printf(Banner, Version)

	// 加载配置
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 初始化eBPF监控
	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		log.Fatalf("eBPF监控初始化失败: %v", err)
	}
	defer monitor.Close()

	// 启动监控
	if err := monitor.Start(); err != nil {
		log.Fatalf("监控启动失败: %v", err)
	}

	if *daemon {
		runDaemon(monitor, cfg)
	} else {
		runInteractive(monitor, cfg)
	}
}

func runInteractive(monitor *ebpf.Monitor, cfg *config.Config) {
	// 初始化终端渲染器
	renderer, err := render.NewTerminalRenderer(cfg)
	if err != nil {
		log.Fatalf("终端渲染器初始化失败: %v", err)
	}
	defer renderer.Close()

	// 设置进程取消回调
	renderer.SetKillProcessCallback(func(containerIndex int) error {
		return monitor.KillContainerProcess(containerIndex)
	})

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动渲染循环
	go renderer.Run(monitor.GetMetrics())

	// 等待退出信号
	<-sigChan
	fmt.Println("\n正在关闭 MicroRadar...")
}

func runDaemon(monitor *ebpf.Monitor, cfg *config.Config) {
	log.Println("MicroRadar 守护进程已启动")
	
	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在守护进程模式下，可以启动Prometheus端点等
	// TODO: 实现Prometheus metrics endpoint

	// 等待退出信号
	<-sigChan
	log.Println("MicroRadar 守护进程正在关闭...")
}

func generateDefaultConfig() {
	defaultConfig := `# MicroRadar 配置文件
monitoring:
  targets:
    - name: "default-cluster"
      runtime: "docker"  # 选项: docker, containerd, cri-o
      metrics:
        - cpu
        - memory
        - network_latency
        - tcp_retransmits
      sampling_rate: "2s"  # 有效值: 1s, 2s, 5s

  alert_thresholds:
    cpu: 70.0          # CPU使用率告警阈值 (%)
    memory: 80.0       # 内存使用率告警阈值 (%)
    network_latency: 10 # 网络延迟告警阈值 (毫秒)

display:
  refresh_rate: "100ms"  # 终端刷新间隔
  theme: "default"       # 主题: default, dark, light

system:
  max_containers: 1000   # 最大监控容器数
  memory_limit: "48MB"   # 内存使用限制
  log_level: "info"      # 日志级别: debug, info, warn, error
`
	fmt.Print(defaultConfig)
}
