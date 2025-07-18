/*
 * MicroRadar 网络性能监控 eBPF 程序
 * 监控容器网络流量、延迟和 TCP 重传
 */

#include "common.h"
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>

/* 网络流量统计映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_NETWORK_FLOWS);
    __type(key, struct flow_key);
    __type(value, struct flow_stats);
} flow_stats_map SEC(".maps");

/* 延迟测量映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_NETWORK_FLOWS);
    __type(key, struct flow_key);
    __type(value, __u64);                  /* 发送时间戳 */
} latency_map SEC(".maps");

/* TCP 连接状态映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_NETWORK_FLOWS);
    __type(key, struct flow_key);
    __type(value, __u32);                  /* TCP 状态 */
} tcp_state_map SEC(".maps");

/* 网络事件环形缓冲区 */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);       /* 512KB 环形缓冲区 */
} network_events SEC(".maps");

/* 网络统计映射表 */
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 20);
    __type(key, __u32);
    __type(value, __u64);
} network_stats_map SEC(".maps");

/* 网络统计索引定义 */
#define NET_STAT_PACKETS_IN      0
#define NET_STAT_PACKETS_OUT     1
#define NET_STAT_BYTES_IN        2
#define NET_STAT_BYTES_OUT       3
#define NET_STAT_TCP_RETRANSMITS 4
#define NET_STAT_UDP_PACKETS     5
#define NET_STAT_LATENCY_SAMPLES 6

/* 辅助函数：解析网络包头 */
static __always_inline int parse_packet(void *data, void *data_end, 
                                       struct flow_key *key, __u32 *packet_size)
{
    struct ethhdr *eth = data;
    
    /* 检查以太网头部 */
    if ((void *)(eth + 1) > data_end)
        return -1;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return -1;
    
    struct iphdr *ip = (struct iphdr *)(eth + 1);
    
    /* 检查 IP 头部 */
    if ((void *)(ip + 1) > data_end)
        return -1;
    
    /* 填充流量键 */
    key->src_ip = ip->saddr;
    key->dst_ip = ip->daddr;
    key->protocol = ip->protocol;
    
    *packet_size = bpf_ntohs(ip->tot_len);
    
    /* 解析传输层协议 */
    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (struct tcphdr *)((char *)ip + (ip->ihl * 4));
        
        if ((void *)(tcp + 1) > data_end)
            return -1;
        
        key->src_port = tcp->source;
        key->dst_port = tcp->dest;
        
        return IPPROTO_TCP;
    } else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (struct udphdr *)((char *)ip + (ip->ihl * 4));
        
        if ((void *)(udp + 1) > data_end)
            return -1;
        
        key->src_port = udp->source;
        key->dst_port = udp->dest;
        
        return IPPROTO_UDP;
    }
    
    return -1;
}

/* 辅助函数：更新网络统计 */
static __always_inline void update_network_stats(__u32 index, __u64 value)
{
    __u64 *count = bpf_map_lookup_elem(&network_stats_map, &index);
    if (count) {
        __sync_fetch_and_add(count, value);
    }
}

/* 辅助函数：获取容器 cgroup ID */
static __always_inline __u64 get_container_cgroup_id(void)
{
    return bpf_get_current_cgroup_id();
}

/* TC 入口：监控入站网络流量 */
SEC("tc/ingress")
int tc_ingress(struct __sk_buff *skb)
{
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    struct flow_key key = {};
    __u32 packet_size = 0;
    
    /* 解析网络包 */
    int proto = parse_packet(data, data_end, &key, &packet_size);
    if (proto < 0)
        return TC_ACT_OK;
    
    /* 获取容器 cgroup ID */
    key.cgroup_id = get_container_cgroup_id();
    if (key.cgroup_id == 0)
        return TC_ACT_OK;
    
    /* 查找或创建流量统计 */
    struct flow_stats *stats = bpf_map_lookup_elem(&flow_stats_map, &key);
    if (!stats) {
        struct flow_stats new_stats = {};
        new_stats.last_seen = bpf_ktime_get_ns();
        new_stats.flags = FLOW_FLAG_INBOUND;
        bpf_map_update_elem(&flow_stats_map, &key, &new_stats, BPF_ANY);
        stats = bpf_map_lookup_elem(&flow_stats_map, &key);
    }
    
    if (stats) {
        __sync_fetch_and_add(&stats->packets, 1);
        __sync_fetch_and_add(&stats->bytes, packet_size);
        stats->last_seen = bpf_ktime_get_ns();
        stats->flags |= FLOW_FLAG_INBOUND;
    }
    
    /* 更新全局统计 */
    update_network_stats(NET_STAT_PACKETS_IN, 1);
    update_network_stats(NET_STAT_BYTES_IN, packet_size);
    
    if (proto == IPPROTO_UDP) {
        update_network_stats(NET_STAT_UDP_PACKETS, 1);
    }
    
    return TC_ACT_OK;
}

/* TC 出口：监控出站网络流量 */
SEC("tc/egress")
int tc_egress(struct __sk_buff *skb)
{
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    struct flow_key key = {};
    __u32 packet_size = 0;

    /* 解析网络包 */
    int proto = parse_packet(data, data_end, &key, &packet_size);
    if (proto < 0)
        return TC_ACT_OK;

    /* 获取容器 cgroup ID */
    key.cgroup_id = get_container_cgroup_id();
    if (key.cgroup_id == 0)
        return TC_ACT_OK;

    /* 记录发送时间戳用于延迟测量 */
    __u64 timestamp = bpf_ktime_get_ns();
    bpf_map_update_elem(&latency_map, &key, &timestamp, BPF_ANY);

    /* 查找或创建流量统计 */
    struct flow_stats *stats = bpf_map_lookup_elem(&flow_stats_map, &key);
    if (!stats) {
        struct flow_stats new_stats = {};
        new_stats.last_seen = timestamp;
        new_stats.flags = FLOW_FLAG_OUTBOUND;
        bpf_map_update_elem(&flow_stats_map, &key, &new_stats, BPF_ANY);
        stats = bpf_map_lookup_elem(&flow_stats_map, &key);
    }

    if (stats) {
        __sync_fetch_and_add(&stats->packets, 1);
        __sync_fetch_and_add(&stats->bytes, packet_size);
        stats->last_seen = timestamp;
        stats->flags |= FLOW_FLAG_OUTBOUND;
    }

    /* 更新全局统计 */
    update_network_stats(NET_STAT_PACKETS_OUT, 1);
    update_network_stats(NET_STAT_BYTES_OUT, packet_size);

    return TC_ACT_OK;
}

/* kprobe：tcp_retransmit_skb - 监控 TCP 重传 */
SEC("kprobe/tcp_retransmit_skb")
int kprobe_tcp_retransmit(struct pt_regs *ctx)
{
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    if (!sk)
        return 0;

    /* 获取连接信息 */
    struct flow_key key = {};
    key.cgroup_id = get_container_cgroup_id();

    if (key.cgroup_id == 0)
        return 0;

    /* 从 socket 结构中提取地址信息 */
    bpf_probe_read_kernel(&key.src_ip, sizeof(key.src_ip), &sk->__sk_common.skc_rcv_saddr);
    bpf_probe_read_kernel(&key.dst_ip, sizeof(key.dst_ip), &sk->__sk_common.skc_daddr);
    bpf_probe_read_kernel(&key.src_port, sizeof(key.src_port), &sk->__sk_common.skc_num);
    bpf_probe_read_kernel(&key.dst_port, sizeof(key.dst_port), &sk->__sk_common.skc_dport);
    key.protocol = IPPROTO_TCP;

    /* 更新重传统计 */
    struct flow_stats *stats = bpf_map_lookup_elem(&flow_stats_map, &key);
    if (stats) {
        __sync_fetch_and_add(&stats->tcp_retransmits, 1);
        stats->flags |= FLOW_FLAG_RETRANSMIT;
    }

    /* 更新全局重传统计 */
    update_network_stats(NET_STAT_TCP_RETRANSMITS, 1);

    /* 发送重传事件 */
    struct event_data *event = bpf_ringbuf_reserve(&network_events, sizeof(*event), 0);
    if (event) {
        event->type = EVENT_NETWORK_PACKET;
        event->timestamp = bpf_ktime_get_ns();
        event->cgroup_id = key.cgroup_id;
        event->pid = bpf_get_current_pid_tgid() >> 32;
        if (stats) {
            __builtin_memcpy(&event->data.network, stats, sizeof(*stats));
        }
        bpf_ringbuf_submit(event, 0);
    }

    return 0;
}

/* tracepoint：tcp_probe - 监控 TCP 连接状态 */
SEC("tracepoint/tcp/tcp_probe")
int trace_tcp_probe(struct trace_event_raw_tcp_probe *ctx)
{
    struct flow_key key = {};
    key.cgroup_id = get_container_cgroup_id();

    if (key.cgroup_id == 0)
        return 0;

    /* 从 tracepoint 参数中获取连接信息 */
    key.src_ip = ctx->saddr;
    key.dst_ip = ctx->daddr;
    key.src_port = ctx->sport;
    key.dst_port = ctx->dport;
    key.protocol = IPPROTO_TCP;

    /* 计算 RTT (往返时间) */
    __u64 *send_time = bpf_map_lookup_elem(&latency_map, &key);
    if (send_time) {
        __u64 current_time = bpf_ktime_get_ns();
        __u64 rtt = current_time - *send_time;

        /* 更新延迟统计 */
        struct flow_stats *stats = bpf_map_lookup_elem(&flow_stats_map, &key);
        if (stats) {
            __sync_fetch_and_add(&stats->latency_sum, rtt);
            __sync_fetch_and_add(&stats->latency_count, 1);
            update_network_stats(NET_STAT_LATENCY_SAMPLES, 1);
        }

        /* 清理延迟映射表 */
        bpf_map_delete_elem(&latency_map, &key);
    }

    return 0;
}

/* 许可证声明 */
char _license[] SEC("license") = "GPL";

/* 内核版本要求 */
__u32 _version SEC("version") = LINUX_VERSION_CODE;
