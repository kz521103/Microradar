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

var (
	Version   = "1.0.0"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	// ASCII 艺术标题
	fmt.Print(`
 __  __ _                 ____           _            
|  \/  (_) ___ _ __ ___   |  _ \ __ _  __| | __ _ _ __ 
| |\/| | |/ __| '__/ _ \  | |_) / _` + "`" + ` |/ _` + "`" + ` |/ _` + "`" + ` | '__|
| |  | | | (__| | | (_) | |  _ < (_| | (_| | (_| | |   
|_|  |_|_|\___|_|  \___/  |_| \_\__,_|\__,_|\__,_|_|   
                                                      
轻量级容器监控工具 v` + Version + `
`)

	// 命令行参数
	var (
		configFile = flag.String("config", "config.yaml", "配置文件路径")
		daemon     = flag.Bool("daemon", false, "以守护进程模式运行")
		version    = flag.Bool("version", false, "显示版本信息")
		init       = flag.Bool("init", false, "生成默认配置文件")
	)
	flag.Parse()

	// 显示版本信息
	if *version {
		fmt.Printf("MicroRadar v%s\n", Version)
		fmt.Printf("Commit: %s\n", Commit)
		fmt.Printf("Build Time: %s\n", BuildTime)
		return
	}

	// 生成默认配置
	if *init {
		if err := config.GenerateDefaultConfig(); err != nil {
			log.Fatalf("生成默认配置失败: %v", err)
		}
		fmt.Println("默认配置已生成到 config.yaml")
		return
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建 eBPF 监控器
	monitor, err := ebpf.NewMonitor(cfg)
	if err != nil {
		log.Fatalf("创建监控器失败: %v", err)
	}
	defer monitor.Close()

	// 启动监控器
	if err := monitor.Start(); err != nil {
		log.Fatalf("启动监控器失败: %v", err)
	}

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if *daemon {
		// 守护进程模式
		runDaemon(cfg, monitor, sigChan)
	} else {
		// 交互模式
		runInteractive(cfg, monitor, sigChan)
	}
}

// runInteractive 运行交互模式
func runInteractive(cfg *config.Config, monitor *ebpf.Monitor, sigChan chan os.Signal) {
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

	// 启动渲染循环
	go renderer.Run(monitor.GetMetrics)

	// 等待退出信号
	<-sigChan
	fmt.Println("\n正在关闭...")
}
