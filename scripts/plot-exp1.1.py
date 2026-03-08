#!/usr/bin/env python3
"""
Exp 1.1: Raft-HT vs Vanilla Raft — Throughput vs Latency
Two figures: P50 (median) and P99 (tail) latency.
Each figure has two subplots: distributed (RTT=50ms) and local (RTT=100ms).
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, 50% weak, Zipfian'

def plot_subplot(ax, rows, rtt_label, percentile='p50'):
    label_suffix = 'P50' if percentile == 'p50' else 'P99'
    s_key = f's_{percentile}'
    w_key = f'w_{percentile}'

    for proto in ['raftht', 'raft']:
        data = extract_tput_latency(rows, proto)
        color = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label = PROTOCOL_LABELS[proto]

        # Strong ops (solid)
        x, y = clean_pairs(data['throughput'], data[s_key])
        ax.plot(x, y, color=color, marker=marker,
                label=f'{label} (strong)', zorder=3)

        # Weak ops (dashed) — only for hybrid protocols
        if proto == 'raftht':
            x, y = clean_pairs(data['throughput'], data[w_key])
            if x:
                ax.plot(x, y, color=WONG['cyan'], marker='o', markersize=5,
                        linestyle='--', label=f'{label} (weak)', zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel(f'{label_suffix} Latency (ms)')
    ax.set_title(f'Raft-HT vs Raft ({rtt_label})\n{WORKLOAD}', fontsize=11)
    ax.legend(loc='upper left')
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

def main():
    base = base_dir()
    dist_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp1.1.csv')
    local_csv = os.path.join(base, 'results', 'eval-local-20260307-final3', 'summary-exp1.1.csv')
    out_dir = os.path.join(base, 'plots')

    setup_style()
    dist_rows = load_csv(dist_csv)
    local_rows = load_csv(local_csv)

    for pct, name_suffix in [('p50', ''), ('p99', '-p99')]:
        fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))
        plot_subplot(ax1, dist_rows, 'RTT = 50 ms', pct)
        plot_subplot(ax2, local_rows, 'RTT = 100 ms', pct)
        plt.tight_layout(w_pad=3)
        save_figure(fig, out_dir, f'exp1.1-throughput-latency{name_suffix}')

if __name__ == '__main__':
    main()
