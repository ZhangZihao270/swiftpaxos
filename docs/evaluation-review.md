# Evaluation Plan Review Notes

Discussion notes on docs/evaluation.md. Work through each point one by one.

---

## 1. Raft-HT story 太薄

当前只有 Exp 1.1 (throughput-vs-latency)，Exp 1.2 (failure recovery) 被注释掉。

建议：
- (a) 恢复 Exp 1.2，展示 H 性质在故障下保持
- (b) 或加 weak 比例 sweep（类似 Exp 3.2），展示 T 性质的另一角度

决定：加 (b) weak 比例 sweep。需要在 evaluation.md 中加 Exp 1.2。

Status: [x] 已决定

---

## 2. EPaxos-HO 缺少冲突率实验

EPaxos 核心特征是 leaderless + 冲突敏感。当前没有 conflict rate sweep。

建议：
- 加 Exp 2.x: 调节 key 分布（uniform -> zipfian -> hotspot），观察 fast path 成功率和延迟

决定：加上。在 evaluation.md 中加 Exp 2.3。

Status: [x] 已决定

---

## 3. Exp 3.2 指标可更丰富

当前只测 strong throughput vs weak 比例。

建议同时展示：
- Strong P50/P99 latency vs weak 比例（更直观展示 T 性质）
- Weak latency vs weak 比例（展示 O 性质优势）
- 双 Y 轴图：strong latency + weak latency vs weak ratio，HO/HT 各一条线

决定：加 strong P50/P99 latency vs weak 比例。在 Exp 3.2 metric 中补充。

Status: [x] 已决定

---

## 4. 缺少 end-to-end application benchmark

纯 YCSB microbenchmark 可能不够 SOSP 审稿人期望。

建议：
- 加简单 application case（如 social feed：profile 更新=strong write，feed 读取=weak read）
- 或用 YCSB realistic 模式（zipfian + read-heavy）

决定：暂不加 application benchmark。改为在 evaluation.md 中明确每个实验的 workload 配置（read/write 比例 + strong/weak 比例 + key 分布），已加入 Workload Configuration 表格。

Status: [x] 已决定

---

## 5. 没有 scalability 实验

当前所有实验固定副本数。

建议：
- Replica count sweep（3, 5, 7）— 展示 CURP-HO 广播开销随副本数增长，而 CURP-HT/Raft-HT 不受影响

决定：暂不加。默认 5 replicas。

Status: [x] 已决定

---

## 6. Cross-case summary 应保留

注释掉的 "single figure comparing all 4 protocols" 非常有价值，能让审稿人快速抓住全貌。

建议：强烈保留。

决定：暂时保留注释状态，之后再决定。

Status: [x] 暂缓

---

## 7. 实验优先级

| 优先级 | 实验 | 理由 |
|--------|------|------|
| P0 | Exp 3.1 + 3.2 | 核心贡献，CURP-HO/HT 已实现 |
| P0 | Exp 1.1 + 1.2 | Raft-HT 已实现，可直接跑 |
| P1 | Cross-case summary | 一张图总结全文，effort 低 |
| P2 | Exp 2.1 | EPaxos-HO 需 port |
| P3 | Exp 2.2 + 2.3 | 需实现 recovery + conflict sweep |

核心观点：Exp 3.2 是整篇论文实验的灵魂，应投入最多精力。Raft-HT 已实现，优先级提升到 P0，可与 CURP 实验并行跑。

Status: [x] 已决定

---

## 8. Speculative read bug 对实现的影响

CurpHT model checking 发现：leader 推测读不能只查 kvStore，需扫描完整 log。
需检查 Go 实现 curp-ht/ 中是否有同样问题。

### 检查结果

**Go 实现确实存在此 bug**，两处：

1. **Leader 推测执行强读** (`curp-ht.go:591-594`):
   `desc.cmd.ComputeResult(r.State)` 只读 kvStore，不扫描 log 中未 apply 的写入。
   如果 PUT(k, v2) 已 append 到 log 但未 commit/apply，推测读返回旧值。

2. **Weak read** (`curp-ht.go:1062-1064`):
   `cmd.ComputeResult(r.State)` 同样只读 kvStore。

**实际影响有限**：
- 推测读结果非最终结果，client 会收到 commit 后的 sync reply 覆盖
- Weak read 本身是弱一致性，读到稍旧的值语义上允许

**修复方案**：在推测读时扫描 log 找最新写入值（类似 TLA+ 的 `SpeculativeVal`）。

Status: [x] 已检查，bug 确认存在，影响有限，待修复
