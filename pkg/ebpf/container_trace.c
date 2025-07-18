/*
 * MicroRadar 容器生命周期跟踪 eBPF 程序
 * 监控容器的创建、启动、停止等生命周期事件
 */

#include "common.h"
#include <linux/sched.h>
#include <linux/cgroup.h>

/* 容器信息映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_CONTAINERS);
    __type(key, __u64);                    /* cgroup_id */
    __type(value, struct container_info);
} container_map SEC(".maps");

/* 进程到容器映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_CONTAINERS * 10);
    __type(key, __u32);                    /* pid */
    __type(value, __u64);                  /* cgroup_id */
} pid_to_cgroup_map SEC(".maps");

/* 事件环形缓冲区 */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);       /* 256KB 环形缓冲区 */
} events SEC(".maps");

/* 统计信息映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 10);
    __type(key, __u32);
    __type(value, __u64);
} stats_map SEC(".maps");

/* 统计索引定义 */
#define STAT_CONTAINERS_CREATED  0
#define STAT_CONTAINERS_STOPPED  1
#define STAT_EVENTS_SENT         2
#define STAT_EVENTS_DROPPED      3

/* 辅助函数：获取当前 cgroup ID */
static __always_inline __u64 get_current_cgroup_id(void)
{
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    if (!task)
        return 0;
    
    return bpf_get_current_cgroup_id();
}

/* 辅助函数：检查是否为容器进程 */
static __always_inline bool is_container_process(__u64 cgroup_id)
{
    /* 简单的启发式检查：cgroup_id 不为 0 且不是 init cgroup */
    return cgroup_id != 0 && cgroup_id != 1;
}

/* 辅助函数：更新统计信息 */
static __always_inline void update_stats(__u32 index)
{
    __u64 *count = bpf_map_lookup_elem(&stats_map, &index);
    if (count) {
        __sync_fetch_and_add(count, 1);
    }
}

/* 辅助函数：发送事件到用户空间 */
static __always_inline int send_event(struct event_data *event)
{
    struct event_data *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        update_stats(STAT_EVENTS_DROPPED);
        return -1;
    }
    
    __builtin_memcpy(e, event, sizeof(*e));
    bpf_ringbuf_submit(e, 0);
    update_stats(STAT_EVENTS_SENT);
    
    return 0;
}

/* 跟踪点：sys_enter_clone - 捕获进程/容器创建 */
SEC("tracepoint/syscalls/sys_enter_clone")
int trace_container_start(struct trace_event_raw_sys_enter* ctx)
{
    __u64 cgroup_id = get_current_cgroup_id();
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    __u32 tid = bpf_get_current_pid_tgid() & 0xFFFFFFFF;
    
    /* 只处理容器进程 */
    if (!is_container_process(cgroup_id))
        return 0;
    
    /* 更新 PID 到 cgroup 映射 */
    bpf_map_update_elem(&pid_to_cgroup_map, &pid, &cgroup_id, BPF_ANY);
    
    /* 检查是否为新容器 */
    struct container_info *existing = bpf_map_lookup_elem(&container_map, &cgroup_id);
    if (existing) {
        /* 容器已存在，只更新进程信息 */
        return 0;
    }
    
    /* 创建新的容器信息 */
    struct container_info container = {};
    container.cgroup_id = cgroup_id;
    container.pid = pid;
    container.ppid = bpf_get_current_pid_tgid() >> 32; /* 父进程 PID */
    container.start_time = bpf_ktime_get_ns();
    container.status = CONTAINER_STATUS_CREATED;
    
    /* 获取进程名 */
    bpf_get_current_comm(container.comm, sizeof(container.comm));
    
    /* 生成容器 ID (简化版本，使用 cgroup_id) */
    __builtin_memset(container.container_id, 0, sizeof(container.container_id));
    bpf_probe_read_kernel_str(container.container_id, 16, &cgroup_id);
    
    /* 存储容器信息 */
    bpf_map_update_elem(&container_map, &cgroup_id, &container, BPF_ANY);
    
    /* 发送容器创建事件 */
    struct event_data event = {};
    event.type = EVENT_CONTAINER_START;
    event.timestamp = bpf_ktime_get_ns();
    event.cgroup_id = cgroup_id;
    event.pid = pid;
    __builtin_memcpy(&event.data.container, &container, sizeof(container));
    
    send_event(&event);
    update_stats(STAT_CONTAINERS_CREATED);
    
    return 0;
}

/* 跟踪点：sys_enter_exit - 捕获进程退出 */
SEC("tracepoint/syscalls/sys_enter_exit")
int trace_container_stop(struct trace_event_raw_sys_enter* ctx)
{
    __u64 cgroup_id = get_current_cgroup_id();
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    if (!is_container_process(cgroup_id))
        return 0;
    
    /* 查找容器信息 */
    struct container_info *container = bpf_map_lookup_elem(&container_map, &cgroup_id);
    if (!container)
        return 0;
    
    /* 检查是否为主进程退出 */
    if (container->pid != pid)
        return 0;
    
    /* 更新容器状态 */
    container->status = CONTAINER_STATUS_STOPPED;
    
    /* 发送容器停止事件 */
    struct event_data event = {};
    event.type = EVENT_CONTAINER_STOP;
    event.timestamp = bpf_ktime_get_ns();
    event.cgroup_id = cgroup_id;
    event.pid = pid;
    __builtin_memcpy(&event.data.container, container, sizeof(*container));
    
    send_event(&event);
    update_stats(STAT_CONTAINERS_STOPPED);
    
    /* 清理映射表 */
    bpf_map_delete_elem(&container_map, &cgroup_id);
    bpf_map_delete_elem(&pid_to_cgroup_map, &pid);
    
    return 0;
}

/* kprobe：cgroup_attach_task - 捕获容器状态变化 */
SEC("kprobe/cgroup_attach_task")
int kprobe_cgroup_attach(struct pt_regs *ctx)
{
    __u64 cgroup_id = get_current_cgroup_id();
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    if (!is_container_process(cgroup_id))
        return 0;
    
    /* 更新 PID 到 cgroup 映射 */
    bpf_map_update_elem(&pid_to_cgroup_map, &pid, &cgroup_id, BPF_ANY);
    
    /* 查找容器信息并更新状态 */
    struct container_info *container = bpf_map_lookup_elem(&container_map, &cgroup_id);
    if (container && container->status == CONTAINER_STATUS_CREATED) {
        container->status = CONTAINER_STATUS_RUNNING;
        
        /* 发送状态变化事件 */
        struct event_data event = {};
        event.type = EVENT_CONTAINER_START;
        event.timestamp = bpf_ktime_get_ns();
        event.cgroup_id = cgroup_id;
        event.pid = pid;
        __builtin_memcpy(&event.data.container, container, sizeof(*container));
        
        send_event(&event);
    }
    
    return 0;
}

/* tracepoint：sched_process_exec - 捕获容器进程执行 */
SEC("tracepoint/sched/sched_process_exec")
int trace_process_exec(struct trace_event_raw_sched_process_exec *ctx)
{
    __u64 cgroup_id = get_current_cgroup_id();
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    if (!is_container_process(cgroup_id))
        return 0;
    
    /* 更新容器信息中的进程名 */
    struct container_info *container = bpf_map_lookup_elem(&container_map, &cgroup_id);
    if (container) {
        bpf_get_current_comm(container->comm, sizeof(container->comm));
        
        /* 如果容器状态为 CREATED，更新为 RUNNING */
        if (container->status == CONTAINER_STATUS_CREATED) {
            container->status = CONTAINER_STATUS_RUNNING;
        }
    }
    
    return 0;
}

/* 用户空间接口：获取容器信息 */
SEC("kprobe/dummy_get_container_info")
int get_container_info(struct pt_regs *ctx)
{
    /* 这个函数主要用于用户空间调用，获取容器信息 */
    return 0;
}

/* 许可证声明 */
char _license[] SEC("license") = "GPL";

/* 内核版本要求 */
__u32 _version SEC("version") = LINUX_VERSION_CODE;
