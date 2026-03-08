#!/usr/bin/env python3
"""
Comprehensive 4-Panel Figure — Distributed Results Only
Panel layout (2×2):
  (a) Strong P50 latency vs throughput (hero left panel)
  (b) Weak P50 latency vs throughput (hero right panel)
  (c) T-property: strong P50 vs weak ratio
  (d) Peak throughput bar chart

This is intended as the main evaluation figure in the paper.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *
import numpy as np

WORKLOAD_TPL = '95/5 R/W, 50% weak, Zipfian'
WORKLOAD_T = '95/5 R/W, t=8, Zipfian'

def get_peak_throughput(rows, protocol):
    filtered = [r for r in rows if r['protocol'] == protocol]
    if not filtered:
        return 0
    return max(float(r['throughput']) for r in filtered)

def extract_weak_ratio_series(rows, protocol):
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['weak_ratio']))
    return {
        'weak_ratio': [int(r['weak_ratio']) for r in filtered],
        's_p50': [float(r['s_p50']) for r in filtered],
        'throughput': [float(r['throughput']) for r in filtered],
    }

def main():
    base = base_dir()
    exp11_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp1.1.csv')
    exp31_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.1.csv')
    exp32_csv = os.path.join(base, 'results', 'eval-dist-20260307-w5', 'summary-exp3.2.csv')
    out_dir = os.path.join(base, 'plots')

    setup_style()
    plt.rcParams.update({'legend.fontsize': 8, 'axes.titlesize': 11})

    exp11_rows = load_csv(exp11_csv)
    exp31_rows = load_csv(exp31_csv)
    exp32_rows = load_csv(exp32_csv)

    fig, ((ax_a, ax_b), (ax_c, ax_d)) = plt.subplots(2, 2, figsize=(12, 9))

    # ── Panel (a): Strong P50 vs Throughput ────────────────────────────
    for proto, src_rows in [('curpho', exp31_rows), ('curpht', exp31_rows),
                             ('curp-baseline', exp31_rows),
                             ('raftht', exp11_rows), ('raft', exp11_rows)]:
        data = extract_tput_latency(src_rows, proto)
        x, y = clean_pairs(data['throughput'], data['s_p50'])
        x, y = pareto_frontier(x, y)
        ax_a.plot(x, y,
                  color=PROTOCOL_COLORS[proto],
                  marker=PROTOCOL_MARKERS[proto],
                  label=PROTOCOL_LABELS[proto], zorder=3)

    ax_a.set_xlabel('Throughput (Kops/sec)')
    ax_a.set_ylabel('Strong P50 Latency (ms)')
    ax_a.set_title(f'(a) Strong Latency vs Throughput\n{WORKLOAD_TPL}')
    ax_a.legend(loc='upper left', fontsize=8)
    ax_a.set_xlim(left=0)
    ax_a.set_ylim(bottom=0)
    ax_a.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

    # ── Panel (b): Weak P50 vs Throughput ──────────────────────────────
    for proto, src_rows in [('curpho', exp31_rows), ('curpht', exp31_rows),
                             ('raftht', exp11_rows)]:
        data = extract_tput_latency(src_rows, proto)
        x, y = clean_pairs(data['throughput'], data['w_p50'])
        x, y = pareto_frontier(x, y)
        ax_b.plot(x, y,
                  color=PROTOCOL_COLORS[proto],
                  marker=PROTOCOL_MARKERS[proto],
                  label=PROTOCOL_LABELS[proto], zorder=3)

    ax_b.set_xlabel('Throughput (Kops/sec)')
    ax_b.set_ylabel('Weak P50 Latency (ms)')
    ax_b.set_title(f'(b) Weak Latency vs Throughput\n{WORKLOAD_TPL}')
    ax_b.legend(loc='upper left', fontsize=8)
    ax_b.set_xlim(left=0)
    ax_b.set_ylim(bottom=0)
    ax_b.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

    # ── Panel (c): T-Property — Strong P50 vs Weak Ratio ──────────────
    for proto in ['raftht', 'curpht', 'curpho']:
        data = extract_weak_ratio_series(exp32_rows, proto)
        ax_c.plot(data['weak_ratio'], data['s_p50'],
                  color=PROTOCOL_COLORS[proto],
                  marker=PROTOCOL_MARKERS[proto],
                  label=PROTOCOL_LABELS[proto], zorder=3)

    ax_c.set_xlabel('Weak Operation Ratio (%)')
    ax_c.set_ylabel('Strong P50 Latency (ms)')
    ax_c.set_title(f'(c) T Property: Strong Latency Stability\n{WORKLOAD_T}')
    ax_c.set_xticks([0, 25, 50, 75, 100])
    ax_c.legend(loc='upper left', fontsize=8)
    ax_c.set_xlim(-5, 105)
    ax_c.set_ylim(bottom=0)

    # ── Panel (d): Peak Throughput Bar Chart ───────────────────────────
    protocols = [
        ('CURP-HO',    'curpho',        exp31_rows),
        ('CURP-HT',    'curpht',        exp31_rows),
        ('Raft-HT',    'raftht',        exp11_rows),
        ('CURP\n(base)', 'curp-baseline', exp31_rows),
        ('Raft',       'raft',           exp11_rows),
    ]

    names = []
    peaks = []
    colors = []
    for label, proto, rows in protocols:
        names.append(label)
        peaks.append(get_peak_throughput(rows, proto) / 1000)
        colors.append(PROTOCOL_COLORS[proto])

    x_pos = np.arange(len(names))
    bars = ax_d.bar(x_pos, peaks, width=0.6, color=colors,
                    edgecolor='white', linewidth=0.5, zorder=3)
    for bar, peak in zip(bars, peaks):
        ax_d.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 1,
                  f'{peak:.0f}K', ha='center', va='bottom', fontsize=9, fontweight='bold')

    ax_d.set_xticks(x_pos)
    ax_d.set_xticklabels(names, fontsize=9)
    ax_d.set_ylabel('Peak Throughput (Kops/sec)')
    ax_d.set_title('(d) Peak Throughput Comparison')
    ax_d.set_ylim(0, max(peaks) * 1.15)
    ax_d.grid(axis='y', alpha=0.3, linestyle='--')

    fig.suptitle('Evaluation Summary — Distributed (RTT = 50 ms)', fontsize=14, fontweight='bold', y=0.98)
    plt.tight_layout(rect=[0, 0, 1, 0.96])
    save_figure(fig, out_dir, 'comprehensive-4panel')

if __name__ == '__main__':
    main()
