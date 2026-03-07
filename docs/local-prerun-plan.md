# Local Cluster Pre-Run Plan

在 local cluster 上预跑所有可执行的实验（排除 EPaxos-HO），验证流程和结果格式。

## 环境

- 3 replicas + 3 clients (co-located), all on localhost
- `networkDelay: 50` (单程 50ms, RTT 100ms, 模拟 geo-distributed)
- 不用 `-d` (distributed mode), 用本地模式

## 前置工作

### Task 1: 创建 local cluster 评估用 config 文件

创建 `eval-local.conf`，基于 `local.conf` 修改：

```
-- Replicas --
replica0 127.0.0.1
replica1 127.0.0.2
replica2 127.0.0.3

-- Clients --
client0 127.0.0.4
client1 127.0.0.5
client2 127.0.0.6

-- Master --
master0 127.0.0.1

masterPort: 7087

// 以下参数由脚本覆盖，这里放默认值
protocol: curpht

// Replica settings
noop:       false
thrifty:    false
fast:       true

// Client settings
reqs:        5000
writes:      5
conflicts:   0
commandSize: 100
clientThreads: 2
pipeline:    true
pendings:    15

// Hybrid settings
weakRatio:   50
weakWrites:  5

// Network delay (单程 50ms = RTT 100ms)
networkDelay: 50

// Key distribution
keySpace:    1000000
zipfSkew:    0.99

// Protocol tuning
maxDescRoutines: 500
batchDelayUs: 150

-- Proxy --
server_alias replica0
client0 (local)
client1 (local)
client2 (local)
---
```

**注意**: config 参数不支持运行时覆盖（没有 CLI flag），所以每个实验需要用 `sed` 动态修改 config 文件再跑。

### Task 2: 改造 run-local.sh 支持 3 clients

当前 `run-local.sh` 只启动 1 个 client。需要创建 `run-local-multi.sh`：
- 启动 master + 3 replicas + 3 clients
- 等待所有 client 完成
- 合并结果（复用 `run-multi-client.sh` 的 Python 合并逻辑）

### Task 3: 创建结果聚合脚本

创建 `scripts/collect-results.sh`:
- 从每个 run 目录提取: throughput, strong P50/P99, weak P50/P99
- 输出 CSV 格式，方便画图
- 格式: `protocol,threads,throughput,s_p50,s_p99,w_p50,w_p99`

---

## 实验脚本

所有结果输出到 `results/eval-local-YYYYMMDD/` 下。

### Script 1: `scripts/eval-exp1.1.sh` — Raft-HT Throughput vs Latency

```bash
PROTOCOLS=("raftht" "raft")
THREAD_COUNTS=(1 2 4 8 16 32)  # local 用较小的 thread count
WEAK_RATIOS=("50" "0")         # raftht=50, raft=0
WRITES=5
WEAK_WRITES=5
```

每个 protocol × thread count 一次 run：
1. `sed` 修改 `eval-local.conf` 中的 `protocol:`, `weakRatio:`, `clientThreads:`
2. 跑 `run-local-multi.sh`
3. 保存到 `results/eval-local-YYYYMMDD/exp1.1/{protocol}/t{threads}/`

| Run | protocol | weakRatio | writes | weakWrites | zipfSkew |
|-----|----------|-----------|--------|------------|----------|
| raftht | raftht | 50 | 5 | 5 | 0.99 |
| raft | raft | 0 | 5 | N/A | 0.99 |

总计: 2 protocols × 6 threads = **12 runs**

### Script 2: `scripts/eval-exp3.1.sh` — CURP Throughput vs Latency

```bash
PROTOCOLS=("curpho" "curpht" "curp")
THREAD_COUNTS=(1 2 4 8 16 32)
```

| Run | protocol | weakRatio | writes | weakWrites | zipfSkew |
|-----|----------|-----------|--------|------------|----------|
| curpho | curpho | 50 | 5 | 5 | 0.99 |
| curpht | curpht | 50 | 5 | 5 | 0.99 |
| curp | curp | 0 | 5 | N/A | 0.99 |

**注意**: Vanilla CURP 用 `protocol: curp` + `weakRatio: 0`（不是 curpht weakRatio=0，那样走的代码路径不同）。

总计: 3 protocols × 6 threads = **18 runs**

### Script 3: `scripts/eval-exp3.2.sh` — T Property Verification

```bash
PROTOCOLS=("raftht" "curpht" "curpho")
WEAK_RATIOS=(0 25 50 75 100)
FIXED_THREADS=8   # local 环境选中等并发
```

| Run | protocol | weakRatio | writes | weakWrites | zipfSkew |
|-----|----------|-----------|--------|------------|----------|
| raftht × 5 | raftht | 0/25/50/75/100 | 50 | 50 | 0.99 |
| curpht × 5 | curpht | 0/25/50/75/100 | 50 | 50 | 0.99 |
| curpho × 5 | curpho | 0/25/50/75/100 | 50 | 50 | 0.99 |

**注意**: Exp 3.2 用 50/50 read/write（`writes: 50, weakWrites: 50`），和 Exp 1.1/3.1 不同。

总计: 3 protocols × 5 weak ratios = **15 runs**

---

## 结果目录结构

```
results/eval-local-YYYYMMDD/
├── exp1.1/
│   ├── raftht/
│   │   ├── t1/  (summary.txt, client*.log, replica*.log)
│   │   ├── t2/
│   │   └── ...
│   └── raft/
│       ├── t1/
│       └── ...
├── exp3.1/
│   ├── curpho/
│   │   ├── t1/
│   │   └── ...
│   ├── curpht/
│   └── curp/
├── exp3.2/
│   ├── raftht/
│   │   ├── w0/   (weakRatio=0)
│   │   ├── w25/
│   │   ├── w50/
│   │   ├── w75/
│   │   └── w100/
│   ├── curpht/
│   └── curpho/
├── summary-exp1.1.csv
├── summary-exp3.1.csv
└── summary-exp3.2.csv
```

## CSV 输出格式

### Exp 1.1 / 3.1 (throughput vs latency):
```csv
protocol,threads,total_threads,throughput,s_avg,s_p50,s_p99,w_avg,w_p50,w_p99
raftht,1,3,1500,102.3,101.5,150.2,51.2,50.8,80.1
raftht,2,6,2900,103.1,102.0,155.3,51.5,51.0,82.0
...
```

### Exp 3.2 (T property):
```csv
protocol,weak_ratio,throughput,s_throughput,s_p50,s_p99,w_throughput,w_p50,w_p99
raftht,0,3000,3000,102.0,155.0,0,N/A,N/A
raftht,25,3100,2325,102.1,155.5,775,51.0,80.0
...
```

## 执行顺序

1. [ ] Task 1: 创建 `eval-local.conf`
2. [ ] Task 2: 创建 `run-local-multi.sh`（支持 3 clients）
3. [ ] Task 3: 创建 `scripts/collect-results.sh`（结果聚合到 CSV）
4. [ ] Script 1: `scripts/eval-exp3.1.sh` — 先跑 CURP（P0 优先级，已验证最多）
5. [ ] Script 2: `scripts/eval-exp3.2.sh` — 再跑 T Property（核心实验）
6. [ ] Script 3: `scripts/eval-exp1.1.sh` — 最后跑 Raft-HT
7. [ ] 检查结果，确认格式正确，数据合理

## 预估时间

- 每次 run: ~30-60s（5000 reqs × 3 clients, local, networkDelay=50ms）
- Exp 1.1: 12 runs × ~45s = ~9 min
- Exp 3.1: 18 runs × ~45s = ~14 min
- Exp 3.2: 15 runs × ~45s = ~11 min
- 总计: ~34 min + 脚本间 cooldown
