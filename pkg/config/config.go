package config

import (
	"fmt"
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 主配置结构
type Config struct {
	Monitoring MonitoringConfig `yaml:"monitoring"`
	Display    DisplayConfig    `yaml:"display"`
	System     SystemConfig     `yaml:"system"`
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Targets         []TargetConfig    `yaml:"targets"`
	AlertThresholds AlertThresholds   `yaml:"alert_thresholds"`
}

// TargetConfig 监控目标配置
type TargetConfig struct {
	Name         string        `yaml:"name"`
	Runtime      string        `yaml:"runtime"`      // docker, containerd, cri-o
	Metrics      []string      `yaml:"metrics"`
	SamplingRate time.Duration `yaml:"sampling_rate"`
}

// AlertThresholds 告警阈值配置
type AlertThresholds struct {
	CPU            float64 `yaml:"cpu"`             // CPU使用率阈值 (%)
	Memory         float64 `yaml:"memory"`          // 内存使用率阈值 (%)
	NetworkLatency float64 `yaml:"network_latency"` // 网络延迟阈值 (ms)
}

// DisplayConfig 显示配置
type DisplayConfig struct {
	RefreshRate time.Duration `yaml:"refresh_rate"`
	Theme       string        `yaml:"theme"`
}

// SystemConfig 系统配置
type SystemConfig struct {
	MaxContainers int    `yaml:"max_containers"`
	MemoryLimit   string `yaml:"memory_limit"`
	LogLevel      string `yaml:"log_level"`
}

// Load 从文件加载配置
func Load(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// 设置默认值
	config.SetDefaults()

	return &config, nil
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if len(c.Monitoring.Targets) == 0 {
		return fmt.Errorf("至少需要配置一个监控目标")
	}

	for i, target := range c.Monitoring.Targets {
		if target.Name == "" {
			return fmt.Errorf("监控目标[%d]名称不能为空", i)
		}

		if target.Runtime == "" {
			return fmt.Errorf("监控目标[%d]运行时不能为空", i)
		}

		validRuntimes := map[string]bool{
			"docker":     true,
			"containerd": true,
			"cri-o":      true,
		}
		if !validRuntimes[target.Runtime] {
			return fmt.Errorf("监控目标[%d]运行时'%s'不支持", i, target.Runtime)
		}

		if target.SamplingRate < time.Second {
			return fmt.Errorf("监控目标[%d]采样间隔不能小于1秒", i)
		}
	}

	// 验证告警阈值
	if c.Monitoring.AlertThresholds.CPU <= 0 || c.Monitoring.AlertThresholds.CPU > 100 {
		return fmt.Errorf("CPU告警阈值必须在0-100之间")
	}

	if c.Monitoring.AlertThresholds.Memory <= 0 || c.Monitoring.AlertThresholds.Memory > 100 {
		return fmt.Errorf("内存告警阈值必须在0-100之间")
	}

	if c.Monitoring.AlertThresholds.NetworkLatency <= 0 {
		return fmt.Errorf("网络延迟告警阈值必须大于0")
	}

	return nil
}

// SetDefaults 设置默认值
func (c *Config) SetDefaults() {
	// 显示配置默认值
	if c.Display.RefreshRate == 0 {
		c.Display.RefreshRate = 100 * time.Millisecond
	}
	if c.Display.Theme == "" {
		c.Display.Theme = "default"
	}

	// 系统配置默认值
	if c.System.MaxContainers == 0 {
		c.System.MaxContainers = 1000
	}
	if c.System.MemoryLimit == "" {
		c.System.MemoryLimit = "48MB"
	}
	if c.System.LogLevel == "" {
		c.System.LogLevel = "info"
	}

	// 监控目标默认值
	for i := range c.Monitoring.Targets {
		if c.Monitoring.Targets[i].SamplingRate == 0 {
			c.Monitoring.Targets[i].SamplingRate = 2 * time.Second
		}
		if len(c.Monitoring.Targets[i].Metrics) == 0 {
			c.Monitoring.Targets[i].Metrics = []string{
				"cpu", "memory", "network_latency", "tcp_retransmits",
			}
		}
	}

	// 告警阈值默认值
	if c.Monitoring.AlertThresholds.CPU == 0 {
		c.Monitoring.AlertThresholds.CPU = 70.0
	}
	if c.Monitoring.AlertThresholds.Memory == 0 {
		c.Monitoring.AlertThresholds.Memory = 80.0
	}
	if c.Monitoring.AlertThresholds.NetworkLatency == 0 {
		c.Monitoring.AlertThresholds.NetworkLatency = 10.0
	}
}

// GetSupportedMetrics 获取支持的指标列表
func GetSupportedMetrics() []string {
	return []string{
		"cpu",
		"memory",
		"network_latency",
		"tcp_retransmits",
		"disk_io",
		"network_io",
	}
}

// GetSupportedRuntimes 获取支持的容器运行时列表
func GetSupportedRuntimes() []string {
	return []string{
		"docker",
		"containerd",
		"cri-o",
	}
}
