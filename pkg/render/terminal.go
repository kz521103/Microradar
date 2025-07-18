package render

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nsf/termbox-go"

	"github.com/kz521103/Microradar/pkg/config"
	"github.com/kz521103/Microradar/pkg/ebpf"
)

// TerminalRenderer 终端渲染器
type TerminalRenderer struct {
	config      *config.Config
	currentView ViewType
	running     bool
	width       int
	height      int
	startTime   time.Time

	// 性能优化
	optimizer   *RenderOptimizer

	// 用户体验
	showHelp    bool
	alertCount  int
	lastError   string

	// 排序和过滤
	sortBy      string
	sortDesc    bool
	filterText  string

	// 进程管理
	selectedIndex       int
	showKillDialog      bool
	killConfirm         bool
	killProcessCallback ProcessKillCallback
	metricsFunc         func() *ebpf.Metrics
}

// ViewType 视图类型
type ViewType int

const (
	ViewContainers ViewType = iota
	ViewNetwork
	ViewSystem
)

// NewTerminalRenderer 创建新的终端渲染器
func NewTerminalRenderer(cfg *config.Config) (*TerminalRenderer, error) {
	if err := termbox.Init(); err != nil {
		return nil, fmt.Errorf("初始化终端失败: %w", err)
	}

	termbox.SetInputMode(termbox.InputEsc)
	termbox.SetOutputMode(termbox.OutputNormal)

	width, height := termbox.Size()

	renderer := &TerminalRenderer{
		config:      cfg,
		currentView: ViewContainers,
		width:       width,
		height:      height,
		startTime:   time.Now(),
		optimizer:   NewRenderOptimizer(15, 100*time.Millisecond), // 15 FPS, 100ms 缓存
		sortBy:      "cpu",
		sortDesc:    true,
	}

	return renderer, nil
}

// Run 运行渲染循环
func (r *TerminalRenderer) Run(metricsFunc func() *ebpf.Metrics) {
	r.running = true
	defer r.Close()

	// 保存指标函数以便在对话框中使用
	r.metricsFunc = metricsFunc

	// 创建事件通道
	eventChan := make(chan termbox.Event)
	go func() {
		for {
			eventChan <- termbox.PollEvent()
		}
	}()

	// 创建刷新定时器
	ticker := time.NewTicker(r.config.Display.RefreshRate)
	defer ticker.Stop()

	for r.running {
		select {
		case ev := <-eventChan:
			if !r.handleEvent(ev) {
				return
			}

		case <-ticker.C:
			// 检查是否需要渲染（帧率控制）
			if !r.optimizer.ShouldRender() {
				continue
			}

			metrics := metricsFunc()
			if metrics != nil {
				r.render(metrics)
			}
		}
	}
}

// Close 关闭渲染器
func (r *TerminalRenderer) Close() {
	r.running = false
	termbox.Close()
}

// handleEvent 处理键盘事件
func (r *TerminalRenderer) handleEvent(ev termbox.Event) bool {
	switch ev.Type {
	case termbox.EventKey:
		switch ev.Key {
		case termbox.KeyEsc, termbox.KeyCtrlC:
			return false // 退出

		case termbox.KeyF1:
			r.toggleHelp()

		case termbox.KeyF2:
			r.switchView()

		case termbox.KeyF5:
			// 强制刷新
			r.optimizer.MarkFullRedraw()
			termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

		case termbox.KeyCtrlL:
			// 清除告警
			r.alertCount = 0
			r.lastError = ""

		case termbox.KeyArrowUp:
			// 向上选择容器
			r.handleUpKey()

		case termbox.KeyArrowDown:
			// 向下选择容器
			r.handleDownKey()

		case termbox.KeySpace:
			// 暂停/恢复刷新
			r.togglePause()

		case termbox.KeyDelete:
			// 显示取消进程对话框
			r.showKillProcessDialog()

		case termbox.KeyEnter:
			// 确认操作
			r.handleEnterKey()

		default:
			switch ev.Ch {
			case 'q', 'Q':
				return false // 退出

			case '1':
				r.currentView = ViewContainers
				r.optimizer.MarkFullRedraw()

			case '2':
				r.currentView = ViewNetwork
				r.optimizer.MarkFullRedraw()

			case '3':
				r.currentView = ViewSystem
				r.optimizer.MarkFullRedraw()

			case 'c', 'C':
				// 按 CPU 排序
				r.setSortBy("cpu")

			case 'm', 'M':
				// 按内存排序
				r.setSortBy("memory")

			case 'n', 'N':
				// 按名称排序
				r.setSortBy("name")

			case 'r', 'R':
				// 反转排序
				r.sortDesc = !r.sortDesc
				r.optimizer.MarkFullRedraw()

			case 'h', 'H':
				// 显示帮助
				r.toggleHelp()

			case 'k', 'K':
				// 显示取消进程对话框
				r.showKillProcessDialog()
			}
		}

	case termbox.EventResize:
		r.width, r.height = termbox.Size()
	}

	return true
}

// render 渲染界面
func (r *TerminalRenderer) render(metrics *ebpf.Metrics) {
	// 检查是否需要全屏重绘
	dirtyRegions, fullRedraw := r.optimizer.GetDirtyRegions()

	if fullRedraw {
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	}

	// 如果显示帮助，只渲染帮助界面
	if r.showHelp {
		r.renderHelpScreen()
		termbox.Flush()
		return
	}

	// 如果显示取消进程对话框
	if r.showKillDialog {
		r.renderKillDialog()
		termbox.Flush()
		return
	}

	// 渲染标题栏
	r.renderHeader(metrics)

	// 根据当前视图渲染内容
	switch r.currentView {
	case ViewContainers:
		r.renderContainers(metrics)
	case ViewNetwork:
		r.renderNetwork(metrics)
	case ViewSystem:
		r.renderSystem(metrics)
	}

	// 渲染状态栏
	r.renderStatusBar()

	// 渲染底部操作栏
	r.renderFooter()

	termbox.Flush()
}

// renderHeader 渲染标题栏
func (r *TerminalRenderer) renderHeader(metrics *ebpf.Metrics) {
	uptime := time.Since(r.startTime)
	header := fmt.Sprintf("[MicroRadar] - PID: %d | Uptime: %s | Containers: %d",
		12345, // TODO: 获取实际 PID
		formatDuration(uptime),
		len(metrics.Containers))

	r.drawText(0, 0, header, termbox.ColorWhite, termbox.ColorBlue)

	// 填充标题栏背景
	for x := len(header); x < r.width; x++ {
		termbox.SetCell(x, 0, ' ', termbox.ColorWhite, termbox.ColorBlue)
	}
}

// renderContainers 渲染容器视图
func (r *TerminalRenderer) renderContainers(metrics *ebpf.Metrics) {
	y := 2

	// 表头
	headers := []string{"CONTAINER", "CPU%", "MEM%", "NET_LAT", "STATUS"}
	widths := []int{20, 8, 8, 10, 12}

	r.drawTableHeader(y, headers, widths)
	y += 2

	// 排序容器
	containers := r.sortContainers(metrics.Containers)

	// 渲染容器数据
	for i, container := range containers {
		if y >= r.height-2 {
			break // 避免超出屏幕
		}

		// 检查是否为选中的容器
		isSelected := (i == r.selectedIndex)
		r.renderContainerRow(y, container, widths, isSelected)
		y++
	}

	// 确保选中索引在有效范围内
	if r.selectedIndex >= len(containers) {
		r.selectedIndex = len(containers) - 1
	}
	if r.selectedIndex < 0 {
		r.selectedIndex = 0
	}
}

// renderContainerRow 渲染容器行
func (r *TerminalRenderer) renderContainerRow(y int, container ebpf.ContainerMetric, widths []int, isSelected bool) {
	x := 0

	// 容器名称
	name := container.Name
	if len(name) > widths[0]-1 {
		name = name[:widths[0]-4] + "..."
	}

	// 设置背景色和前景色
	bg := termbox.ColorDefault
	fg := termbox.ColorDefault

	// 如果是选中的行，使用高亮背景
	if isSelected {
		bg = termbox.ColorBlue
		fg = termbox.ColorWhite
	}

	r.drawText(x, y, name, fg, bg)
	x += widths[0]

	// CPU 使用率
	cpuColor := fg
	if container.CPUPercent >= r.config.Monitoring.AlertThresholds.CPU {
		cpuColor = termbox.ColorYellow
		if isSelected {
			cpuColor = termbox.ColorYellow | termbox.AttrBold
		}
	}
	cpuText := fmt.Sprintf("%.1f", container.CPUPercent)
	r.drawText(x, y, cpuText, cpuColor, bg)
	x += widths[1]

	// 内存使用率
	memColor := fg
	if container.MemoryPercent >= r.config.Monitoring.AlertThresholds.Memory {
		memColor = termbox.ColorRed
		if isSelected {
			memColor = termbox.ColorRed | termbox.AttrBold
		}
	}
	memText := fmt.Sprintf("%.1f", container.MemoryPercent)
	r.drawText(x, y, memText, memColor, bg)
	x += widths[2]

	// 网络延迟
	latColor := fg
	latText := fmt.Sprintf("%.0fms", container.NetworkLatency)
	if container.NetworkLatency >= r.config.Monitoring.AlertThresholds.NetworkLatency {
		latColor = termbox.ColorYellow
		if isSelected {
			latColor = termbox.ColorYellow | termbox.AttrBold
		}
		latText += " ⚠️"
	}
	r.drawText(x, y, latText, latColor, bg)
	x += widths[3]

	// 状态
	statusColor := termbox.ColorGreen
	if container.Status != "running" {
		statusColor = termbox.ColorRed
	}
	if isSelected {
		statusColor |= termbox.AttrBold
	}
	r.drawText(x, y, container.Status, statusColor, bg)
}

// renderNetwork 渲染网络视图
func (r *TerminalRenderer) renderNetwork(metrics *ebpf.Metrics) {
	y := 2

	// 表头
	headers := []string{"CONTAINER", "PACKETS_IN", "PACKETS_OUT", "BYTES_IN", "BYTES_OUT", "LATENCY", "RETRANS"}
	widths := []int{15, 12, 12, 12, 12, 10, 8}

	r.drawTableHeader(y, headers, widths)
	y += 2

	// 渲染网络数据 (模拟数据)
	networkData := []struct {
		name       string
		packetsIn  uint64
		packetsOut uint64
		bytesIn    uint64
		bytesOut   uint64
		latency    float64
		retrans    uint32
	}{
		{"web-server", 15420, 12350, 2048576, 1536000, 8.5, 0},
		{"db-primary", 8960, 7840, 1024000, 896000, 12.3, 2},
		{"cache-redis", 25600, 23400, 512000, 480000, 3.2, 0},
	}

	for i, data := range networkData {
		if y >= r.height-2 {
			break
		}

		r.renderNetworkRow(y, data.name, data.packetsIn, data.packetsOut,
			data.bytesIn, data.bytesOut, data.latency, data.retrans, widths)
		y++
	}
}

// renderNetworkRow 渲染网络行
func (r *TerminalRenderer) renderNetworkRow(y int, name string, packetsIn, packetsOut,
	bytesIn, bytesOut uint64, latency float64, retrans uint32, widths []int) {
	x := 0

	// 容器名称
	if len(name) > widths[0]-1 {
		name = name[:widths[0]-4] + "..."
	}
	r.drawText(x, y, name, termbox.ColorDefault, termbox.ColorDefault)
	x += widths[0]

	// 入站包数
	r.drawText(x, y, fmt.Sprintf("%d", packetsIn), termbox.ColorDefault, termbox.ColorDefault)
	x += widths[1]

	// 出站包数
	r.drawText(x, y, fmt.Sprintf("%d", packetsOut), termbox.ColorDefault, termbox.ColorDefault)
	x += widths[2]

	// 入站字节数
	r.drawText(x, y, r.formatBytes(bytesIn), termbox.ColorDefault, termbox.ColorDefault)
	x += widths[3]

	// 出站字节数
	r.drawText(x, y, r.formatBytes(bytesOut), termbox.ColorDefault, termbox.ColorDefault)
	x += widths[4]

	// 延迟
	latColor := termbox.ColorDefault
	latText := fmt.Sprintf("%.1fms", latency)
	if latency >= r.config.Monitoring.AlertThresholds.NetworkLatency {
		latColor = termbox.ColorYellow
		latText += " ⚠️"
	}
	r.drawText(x, y, latText, latColor, termbox.ColorDefault)
	x += widths[5]

	// 重传次数
	retransColor := termbox.ColorDefault
	if retrans > 0 {
		retransColor = termbox.ColorRed
	}
	r.drawText(x, y, fmt.Sprintf("%d", retrans), retransColor, termbox.ColorDefault)
}

// renderSystem 渲染系统视图
func (r *TerminalRenderer) renderSystem(metrics *ebpf.Metrics) {
	y := 2

	// 系统概览
	r.drawText(0, y, "系统概览", termbox.ColorWhite|termbox.AttrBold, termbox.ColorDefault)
	y += 2

	// 系统指标
	systemInfo := []struct {
		label string
		value string
		color termbox.Attribute
	}{
		{"总容器数:", fmt.Sprintf("%d", len(metrics.Containers)), termbox.ColorDefault},
		{"运行中:", fmt.Sprintf("%d", r.countRunningContainers(metrics)), termbox.ColorGreen},
		{"eBPF Maps:", fmt.Sprintf("%d", metrics.EBPFMapsCount), termbox.ColorDefault},
		{"系统内存:", r.formatBytes(metrics.SystemMemory), termbox.ColorDefault},
		{"最后更新:", metrics.LastUpdate.Format("15:04:05"), termbox.ColorDefault},
	}

	for _, info := range systemInfo {
		r.drawText(0, y, info.label, termbox.ColorDefault, termbox.ColorDefault)
		r.drawText(15, y, info.value, info.color, termbox.ColorDefault)
		y++
	}

	y += 2

	// 资源使用统计
	r.drawText(0, y, "资源使用统计", termbox.ColorWhite|termbox.AttrBold, termbox.ColorDefault)
	y += 2

	// CPU 使用率分布
	r.drawText(0, y, "CPU 使用率分布:", termbox.ColorDefault, termbox.ColorDefault)
	y++
	cpuDistribution := r.calculateCPUDistribution(metrics)
	for threshold, count := range cpuDistribution {
		color := termbox.ColorDefault
		if threshold >= 70 {
			color = termbox.ColorRed
		} else if threshold >= 50 {
			color = termbox.ColorYellow
		}
		r.drawText(2, y, fmt.Sprintf("%d%%-: %d 容器", threshold, count), color, termbox.ColorDefault)
		y++
	}

	y++

	// 内存使用率分布
	r.drawText(0, y, "内存使用率分布:", termbox.ColorDefault, termbox.ColorDefault)
	y++
	memoryDistribution := r.calculateMemoryDistribution(metrics)
	for threshold, count := range memoryDistribution {
		color := termbox.ColorDefault
		if threshold >= 80 {
			color = termbox.ColorRed
		} else if threshold >= 60 {
			color = termbox.ColorYellow
		}
		r.drawText(2, y, fmt.Sprintf("%d%%-: %d 容器", threshold, count), color, termbox.ColorDefault)
		y++
	}

	y += 2

	// 告警统计
	r.drawText(0, y, "告警统计", termbox.ColorWhite|termbox.AttrBold, termbox.ColorDefault)
	y += 2

	alerts := r.calculateAlerts(metrics)
	alertInfo := []struct {
		label string
		count int
		color termbox.Attribute
	}{
		{"CPU 告警:", alerts.cpu, termbox.ColorYellow},
		{"内存告警:", alerts.memory, termbox.ColorRed},
		{"网络告警:", alerts.network, termbox.ColorYellow},
	}

	for _, alert := range alertInfo {
		color := termbox.ColorDefault
		if alert.count > 0 {
			color = alert.color
		}
		r.drawText(0, y, alert.label, termbox.ColorDefault, termbox.ColorDefault)
		r.drawText(15, y, fmt.Sprintf("%d", alert.count), color, termbox.ColorDefault)
		y++
	}
}

// renderFooter 渲染底部状态栏
func (r *TerminalRenderer) renderFooter() {
	y := r.height - 1
	footer := "[1] Containers [2] Network [3] System [↑/↓] Select [K] Kill [F1] Help [Q] Quit"

	r.drawText(0, y, footer, termbox.ColorWhite, termbox.ColorBlue)

	// 填充底部栏背景
	for x := len(footer); x < r.width; x++ {
		termbox.SetCell(x, y, ' ', termbox.ColorWhite, termbox.ColorBlue)
	}
}

// drawTableHeader 绘制表格标题
func (r *TerminalRenderer) drawTableHeader(y int, headers []string, widths []int) {
	x := 0
	for i, header := range headers {
		r.drawText(x, y, header, termbox.ColorWhite, termbox.ColorDefault)
		x += widths[i]
	}

	// 绘制分隔线
	x = 0
	for i, width := range widths {
		line := strings.Repeat("─", width-1)
		r.drawText(x, y+1, line, termbox.ColorDefault, termbox.ColorDefault)
		x += width
	}
}

// drawText 绘制文本
func (r *TerminalRenderer) drawText(x, y int, text string, fg, bg termbox.Attribute) {
	for i, ch := range text {
		if x+i >= r.width {
			break
		}
		termbox.SetCell(x+i, y, ch, fg, bg)
	}
}

// switchView 切换视图
func (r *TerminalRenderer) switchView() {
	r.currentView = (r.currentView + 1) % 3
}

// showHelp 显示帮助信息
func (r *TerminalRenderer) showHelp() {
	// 清屏
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	// 帮助标题
	title := "MicroRadar 帮助信息"
	titleX := (r.width - len(title)) / 2
	r.drawText(titleX, 2, title, termbox.ColorWhite|termbox.AttrBold, termbox.ColorDefault)

	y := 4

	// 快捷键说明
	helpItems := []struct {
		key   string
		desc  string
		color termbox.Attribute
	}{
		{"快捷键操作:", "", termbox.ColorWhite | termbox.AttrBold},
		{"", "", termbox.ColorDefault},
		{"1", "切换到容器视图", termbox.ColorGreen},
		{"2", "切换到网络视图", termbox.ColorGreen},
		{"3", "切换到系统视图", termbox.ColorGreen},
		{"", "", termbox.ColorDefault},
		{"F1", "显示/隐藏帮助", termbox.ColorCyan},
		{"F2", "循环切换视图", termbox.ColorCyan},
		{"F5", "强制刷新", termbox.ColorCyan},
		{"", "", termbox.ColorDefault},
		{"↑/↓", "选择容器", termbox.ColorGreen},
		{"K / Del", "取消选中的容器进程", termbox.ColorRed},
		{"Enter", "确认操作", termbox.ColorGreen},
		{"", "", termbox.ColorDefault},
		{"Ctrl+L", "清除警告", termbox.ColorYellow},
		{"Q / Esc", "退出程序", termbox.ColorRed},
		{"", "", termbox.ColorDefault},
		{"告警标识:", "", termbox.ColorWhite | termbox.AttrBold},
		{"", "", termbox.ColorDefault},
		{"⚠️", "CPU ≥ 70% 或网络延迟 ≥ 10ms", termbox.ColorYellow},
		{"🔴", "内存使用率 ≥ 80%", termbox.ColorRed},
		{"⚡", "网络异常或高延迟", termbox.ColorYellow},
		{"", "", termbox.ColorDefault},
		{"视图说明:", "", termbox.ColorWhite | termbox.AttrBold},
		{"", "", termbox.ColorDefault},
		{"容器视图", "显示所有容器的 CPU、内存、网络状态", termbox.ColorDefault},
		{"网络视图", "显示网络流量、延迟、重传统计", termbox.ColorDefault},
		{"系统视图", "显示系统概览和资源分布", termbox.ColorDefault},
	}

	for _, item := range helpItems {
		if item.key == "" && item.desc == "" {
			y++
			continue
		}

		if item.key != "" {
			r.drawText(5, y, item.key+":", termbox.ColorWhite, termbox.ColorDefault)
			r.drawText(20, y, item.desc, item.color, termbox.ColorDefault)
		} else {
			r.drawText(5, y, item.desc, item.color, termbox.ColorDefault)
		}
		y++

		if y >= r.height-3 {
			break
		}
	}

	// 底部提示
	footer := "按任意键返回"
	footerX := (r.width - len(footer)) / 2
	r.drawText(footerX, r.height-2, footer, termbox.ColorWhite, termbox.ColorBlue)

	termbox.Flush()

	// 等待按键
	termbox.PollEvent()
}

// renderHelpScreen 渲染帮助屏幕
func (r *TerminalRenderer) renderHelpScreen() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	r.showHelp()
}

// renderKillDialog 渲染取消进程对话框
func (r *TerminalRenderer) renderKillDialog() {
	// 保存当前屏幕内容
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	// 获取对话框尺寸和位置
	dialogWidth := 50
	dialogHeight := 8
	dialogX := (r.width - dialogWidth) / 2
	dialogY := (r.height - dialogHeight) / 2

	// 绘制对话框边框
	for y := dialogY; y < dialogY+dialogHeight; y++ {
		for x := dialogX; x < dialogX+dialogWidth; x++ {
			// 边框
			if y == dialogY || y == dialogY+dialogHeight-1 ||
			   x == dialogX || x == dialogX+dialogWidth-1 {
				termbox.SetCell(x, y, ' ', termbox.ColorWhite, termbox.ColorBlue)
			} else {
				termbox.SetCell(x, y, ' ', termbox.ColorDefault, termbox.ColorDefault)
			}
		}
	}

	// 获取选中的容器名称
	containerName := "未知容器"
	if r.selectedIndex >= 0 && r.metricsFunc != nil {
		metrics := r.metricsFunc()
		if metrics != nil && r.selectedIndex < len(metrics.Containers) {
			containerName = metrics.Containers[r.selectedIndex].Name
		}
	}

	// 绘制标题
	title := "取消进程确认"
	titleX := dialogX + (dialogWidth - len(title)) / 2
	r.drawText(titleX, dialogY, title, termbox.ColorWhite, termbox.ColorBlue)

	// 绘制内容
	message := fmt.Sprintf("确定要取消容器 '%s' 的进程吗?", containerName)
	messageX := dialogX + (dialogWidth - len(message)) / 2
	r.drawText(messageX, dialogY+2, message, termbox.ColorWhite, termbox.ColorDefault)

	// 绘制警告
	warning := "警告: 这将强制终止容器中的所有进程!"
	warningX := dialogX + (dialogWidth - len(warning)) / 2
	r.drawText(warningX, dialogY+3, warning, termbox.ColorRed, termbox.ColorDefault)

	// 绘制按钮
	yesText := "是"
	noText := "否"
	buttonY := dialogY + 5

	// 是按钮
	yesX := dialogX + dialogWidth/3 - len(yesText)/2
	yesBg := termbox.ColorDefault
	yesFg := termbox.ColorDefault
	if r.killConfirm {
		yesBg = termbox.ColorRed
		yesFg = termbox.ColorWhite
	}
	r.drawText(yesX, buttonY, yesText, yesFg, yesBg)

	// 否按钮
	noX := dialogX + 2*dialogWidth/3 - len(noText)/2
	noBg := termbox.ColorDefault
	noFg := termbox.ColorDefault
	if !r.killConfirm {
		noBg = termbox.ColorBlue
		noFg = termbox.ColorWhite
	}
	r.drawText(noX, buttonY, noText, noFg, noBg)

	// 绘制提示
	hint := "使用 ↑/↓ 键选择，Enter 确认"
	hintX := dialogX + (dialogWidth - len(hint)) / 2
	r.drawText(hintX, dialogY+dialogHeight-2, hint, termbox.ColorWhite, termbox.ColorBlue)
}

// formatDuration 格式化时间间隔
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// formatBytes 格式化字节数
func (r *TerminalRenderer) formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// countRunningContainers 统计运行中的容器数量
func (r *TerminalRenderer) countRunningContainers(metrics *ebpf.Metrics) int {
	count := 0
	for _, container := range metrics.Containers {
		if container.Status == "running" {
			count++
		}
	}
	return count
}

// calculateCPUDistribution 计算 CPU 使用率分布
func (r *TerminalRenderer) calculateCPUDistribution(metrics *ebpf.Metrics) map[int]int {
	distribution := map[int]int{
		0:  0, // 0-29%
		30: 0, // 30-49%
		50: 0, // 50-69%
		70: 0, // 70-89%
		90: 0, // 90-100%
	}

	for _, container := range metrics.Containers {
		cpu := int(container.CPUPercent)
		switch {
		case cpu < 30:
			distribution[0]++
		case cpu < 50:
			distribution[30]++
		case cpu < 70:
			distribution[50]++
		case cpu < 90:
			distribution[70]++
		default:
			distribution[90]++
		}
	}

	return distribution
}

// calculateMemoryDistribution 计算内存使用率分布
func (r *TerminalRenderer) calculateMemoryDistribution(metrics *ebpf.Metrics) map[int]int {
	distribution := map[int]int{
		0:  0, // 0-39%
		40: 0, // 40-59%
		60: 0, // 60-79%
		80: 0, // 80-100%
	}

	for _, container := range metrics.Containers {
		memory := int(container.MemoryPercent)
		switch {
		case memory < 40:
			distribution[0]++
		case memory < 60:
			distribution[40]++
		case memory < 80:
			distribution[60]++
		default:
			distribution[80]++
		}
	}

	return distribution
}

// AlertCounts 告警计数
type AlertCounts struct {
	cpu     int
	memory  int
	network int
}

// calculateAlerts 计算告警数量
func (r *TerminalRenderer) calculateAlerts(metrics *ebpf.Metrics) AlertCounts {
	var alerts AlertCounts

	for _, container := range metrics.Containers {
		if container.CPUPercent >= r.config.Monitoring.AlertThresholds.CPU {
			alerts.cpu++
		}
		if container.MemoryPercent >= r.config.Monitoring.AlertThresholds.Memory {
			alerts.memory++
		}
		if container.NetworkLatency >= r.config.Monitoring.AlertThresholds.NetworkLatency {
			alerts.network++
		}
	}

	return alerts
}

// toggleHelp 切换帮助显示
func (r *TerminalRenderer) toggleHelp() {
	r.showHelp = !r.showHelp
	r.optimizer.MarkFullRedraw()
}

// handleUpKey 处理向上键
func (r *TerminalRenderer) handleUpKey() {
	if r.showKillDialog {
		// 在取消对话框中切换选项
		r.killConfirm = !r.killConfirm
	} else {
		// 在容器列表中向上选择
		if r.selectedIndex > 0 {
			r.selectedIndex--
		}
	}
	r.optimizer.MarkFullRedraw()
}

// handleDownKey 处理向下键
func (r *TerminalRenderer) handleDownKey() {
	if r.showKillDialog {
		// 在取消对话框中切换选项
		r.killConfirm = !r.killConfirm
	} else {
		// 在容器列表中向下选择
		r.selectedIndex++
	}
	r.optimizer.MarkFullRedraw()
}

// handleEnterKey 处理回车键
func (r *TerminalRenderer) handleEnterKey() {
	if r.showKillDialog {
		if r.killConfirm {
			// 执行取消进程操作
			r.killSelectedProcess()
		}
		r.showKillDialog = false
		r.killConfirm = false
		r.optimizer.MarkFullRedraw()
	}
}

// showKillProcessDialog 显示取消进程对话框
func (r *TerminalRenderer) showKillProcessDialog() {
	if r.currentView == ViewContainers {
		r.showKillDialog = true
		r.killConfirm = false
		r.optimizer.MarkFullRedraw()
	}
}

// killSelectedProcess 取消选中的进程
func (r *TerminalRenderer) killSelectedProcess() {
	// 这里需要获取当前的容器列表
	// 由于我们需要访问监控数据，这个功能需要通过回调实现
	if r.killProcessCallback != nil {
		r.killProcessCallback(r.selectedIndex)
	}
}

// ProcessKillCallback 进程取消回调函数类型
type ProcessKillCallback func(containerIndex int) error

// SetKillProcessCallback 设置进程取消回调
func (r *TerminalRenderer) SetKillProcessCallback(callback ProcessKillCallback) {
	r.killProcessCallback = callback
}

// togglePause 切换暂停状态
func (r *TerminalRenderer) togglePause() {
	// 这里可以实现暂停/恢复功能
	r.optimizer.MarkFullRedraw()
}

// setSortBy 设置排序方式
func (r *TerminalRenderer) setSortBy(sortBy string) {
	if r.sortBy == sortBy {
		r.sortDesc = !r.sortDesc
	} else {
		r.sortBy = sortBy
		r.sortDesc = true
	}
	r.optimizer.MarkFullRedraw()
}

// sortContainers 排序容器
func (r *TerminalRenderer) sortContainers(containers []ebpf.ContainerMetric) []ebpf.ContainerMetric {
	sorted := make([]ebpf.ContainerMetric, len(containers))
	copy(sorted, containers)

	sort.Slice(sorted, func(i, j int) bool {
		var less bool
		switch r.sortBy {
		case "cpu":
			less = sorted[i].CPUPercent < sorted[j].CPUPercent
		case "memory":
			less = sorted[i].MemoryPercent < sorted[j].MemoryPercent
		case "name":
			less = sorted[i].Name < sorted[j].Name
		case "latency":
			less = sorted[i].NetworkLatency < sorted[j].NetworkLatency
		default:
			less = sorted[i].CPUPercent < sorted[j].CPUPercent
		}

		if r.sortDesc {
			return !less
		}
		return less
	})

	return sorted
}

// renderStatusBar 渲染状态栏
func (r *TerminalRenderer) renderStatusBar() {
	y := r.height - 2

	// 状态信息
	status := fmt.Sprintf("FPS: %.1f | Sort: %s %s | View: %s",
		r.optimizer.GetCurrentFPS(),
		r.sortBy,
		map[bool]string{true: "↓", false: "↑"}[r.sortDesc],
		[]string{"Containers", "Network", "System"}[r.currentView])

	if r.lastError != "" {
		status += fmt.Sprintf(" | Error: %s", r.lastError)
	}

	if r.alertCount > 0 {
		status += fmt.Sprintf(" | Alerts: %d", r.alertCount)
	}

	r.drawText(0, y, status, termbox.ColorWhite, termbox.ColorBlack)

	// 填充状态栏背景
	for x := len(status); x < r.width; x++ {
		termbox.SetCell(x, y, ' ', termbox.ColorWhite, termbox.ColorBlack)
	}
}
