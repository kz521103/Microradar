/*
 * MicroRadar eBPF 通用头文件
 * 定义共享的数据结构和常量
 */

#ifndef __MICRORADAR_COMMON_H__
#define __MICRORADAR_COMMON_H__

#include <linux/types.h>
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* 版本信息 */
#define MICRORADAR_VERSION_MAJOR 1
#define MICRORADAR_VERSION_MINOR 0
#define MICRORADAR_VERSION_PATCH 0

/* 常量定义 */
#define MAX_CONTAINERS 1000
#define MAX_COMM_LEN 16
#define MAX_CONTAINER_ID_LEN 64
#define MAX_NETWORK_FLOWS 10240

/* 容器信息结构 */
struct container_info {
    __u64 cgroup_id;                    /* cgroup ID */
    __u32 pid;                          /* 进程 ID */
    __u32 ppid;                         /* 父进程 ID */
    char container_id[MAX_CONTAINER_ID_LEN]; /* 容器 ID */
    char comm[MAX_COMM_LEN];            /* 进程名 */
    __u64 start_time;                   /* 启动时间 (纳秒) */
    __u32 cpu_usage;                    /* CPU 使用率 (千分比) */
    __u64 memory_usage;                 /* 内存使用量 (字节) */
    __u32 status;                       /* 容器状态 */
};

/* 网络流量键 */
struct flow_key {
    __u32 src_ip;                       /* 源 IP 地址 */
    __u32 dst_ip;                       /* 目标 IP 地址 */
    __u16 src_port;                     /* 源端口 */
    __u16 dst_port;                     /* 目标端口 */
    __u8 protocol;                      /* 协议 (TCP/UDP) */
    __u8 pad[3];                        /* 填充对齐 */
    __u64 cgroup_id;                    /* 关联的 cgroup ID */
};

/* 网络流量统计 */
struct flow_stats {
    __u64 packets;                      /* 数据包数量 */
    __u64 bytes;                        /* 字节数 */
    __u64 latency_sum;                  /* 延迟总和 (纳秒) */
    __u32 latency_count;                /* 延迟测量次数 */
    __u64 last_seen;                    /* 最后见到时间 */
    __u32 tcp_retransmits;              /* TCP 重传次数 */
    __u32 flags;                        /* 标志位 */
};

/* 系统事件类型 */
enum event_type {
    EVENT_CONTAINER_START = 1,
    EVENT_CONTAINER_STOP = 2,
    EVENT_NETWORK_PACKET = 3,
    EVENT_CPU_SAMPLE = 4,
    EVENT_MEMORY_SAMPLE = 5,
};

/* 事件数据结构 */
struct event_data {
    __u32 type;                         /* 事件类型 */
    __u64 timestamp;                    /* 时间戳 */
    __u64 cgroup_id;                    /* cgroup ID */
    __u32 pid;                          /* 进程 ID */
    union {
        struct container_info container;
        struct flow_stats network;
        __u64 value;                    /* 通用数值 */
    } data;
};

/* 容器状态定义 */
#define CONTAINER_STATUS_UNKNOWN    0
#define CONTAINER_STATUS_CREATED    1
#define CONTAINER_STATUS_RUNNING    2
#define CONTAINER_STATUS_PAUSED     3
#define CONTAINER_STATUS_STOPPED    4
#define CONTAINER_STATUS_EXITED     5

/* 网络协议定义 */
#define IPPROTO_TCP 6
#define IPPROTO_UDP 17

/* 标志位定义 */
#define FLOW_FLAG_INBOUND   0x01
#define FLOW_FLAG_OUTBOUND  0x02
#define FLOW_FLAG_RETRANSMIT 0x04

/* 辅助宏 */
#define SEC(name) __attribute__((section(name), used))

/* 许可证声明 */
char _license[] SEC("license") = "GPL";

/* 内核版本要求 */
__u32 _version SEC("version") = LINUX_VERSION_CODE;

#endif /* __MICRORADAR_COMMON_H__ */
