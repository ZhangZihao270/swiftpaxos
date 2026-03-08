#!/usr/bin/env python3
"""
Exp 3.1 (5-Replica): CURP-HO vs CURP-HT vs Baseline — Throughput vs Latency
Single-panel figure for 5-replica distributed results (RTT=50ms).
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, 50% weak, Zipfian, 5 replicas'

def plot_figure(ax, rows, percentile='p50'):
    label_suffix = 'P50' if percentile == 'p50' else 'P99'
    s_key = f's_{percentile}'
    w_key = f'w_{percentile}'

    for proto in ['curpho', 'curpht', 'curp-baseline']:
        data = extract_tput_latency(rows, proto)
        color = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label = PROTOCOL_LABELS[proto]

        # Strong ops (solid) — trim past peak throughput
        x, y = clean_pairs(data['throughput'], data[s_key])
        x, y = pareto_frontier(x, y)
        ax.plot(x, y, color=color, marker=marker,
                label=f'{label} (strong)', zorder=3)

        # Weak ops (dashed) — only for hybrid protocols
        if proto != 'curp-baseline':
            x, y = clean_pairs(data['throughput'], data[w_key])
            x, y = pareto_frontier(x, y)
            if x:
                ax.plot(x, y, color=color, marker=marker, markersize=5,
                        linestyle='--', alpha=0.7,
                        label=f'{label} (weak)', zorder=2)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel(f'{label_suffix} Latency (ms)')
    ax.set_title(f'CURP Throughput vs Latency (5 replicas, RTT = 50 ms)\n{WORKLOAD}', fontsize=11)
    ax.legend(loc='upper left', fontsize=8, ncol=1)
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'eval-5r-20260308', 'summary-exp3.1.csv')
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    for pct, name_suffix in [('p50', ''), ('p99', '-p99')]:
        fig, ax = plt.subplots(1, 1, figsize=(7, 4.5))
        plot_figure(ax, rows, pct)
        plt.tight_layout()
        save_figure(fig, out_dir, f'exp3.1-5r-throughput-latency{name_suffix}')

if __name__ == '__main__':
    main()
