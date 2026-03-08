#!/usr/bin/env python3
"""
Exp 3.2: T Property Verification — Strong Latency Stability
Two figures: strong P50 latency and throughput vs weak ratio.
Each figure has two subplots: distributed (RTT=50ms) and local (RTT=100ms).
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD_DIST = '95/5 R/W, t=8, Zipfian'
WORKLOAD_LOCAL = '50/50 R/W, t=8, Zipfian'

def extract_weak_ratio_series(rows, protocol):
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['weak_ratio']))
    return {
        'weak_ratio': [int(r['weak_ratio']) for r in filtered],
        's_p50': [float(r['s_p50']) for r in filtered],
        's_p99': [float(r['s_p99']) for r in filtered],
        'throughput': [float(r['throughput']) for r in filtered],
    }

def plot_latency_subplot(ax, rows, rtt_label, workload):
    for proto in ['raftht', 'curpht', 'curpho']:
        data = extract_weak_ratio_series(rows, proto)
        ax.plot(data['weak_ratio'], data['s_p50'],
                color=PROTOCOL_COLORS[proto], marker=PROTOCOL_MARKERS[proto],
                label=f'{PROTOCOL_LABELS[proto]} (P50)', zorder=3)
        ax.plot(data['weak_ratio'], data['s_p99'],
                color=PROTOCOL_COLORS[proto], marker=PROTOCOL_MARKERS[proto],
                markersize=5, linestyle=':', alpha=0.6,
                label=f'{PROTOCOL_LABELS[proto]} (P99)', zorder=2)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Strong Latency (ms)')
    ax.set_title(f'T Property: Strong Latency ({rtt_label})\n{workload}', fontsize=11)
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left', fontsize=7.5, ncol=2)
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)

def plot_throughput_subplot(ax, rows, rtt_label, workload):
    for proto in ['raftht', 'curpht', 'curpho']:
        data = extract_weak_ratio_series(rows, proto)
        ax.plot(data['weak_ratio'], [t/1000 for t in data['throughput']],
                color=PROTOCOL_COLORS[proto], marker=PROTOCOL_MARKERS[proto],
                label=PROTOCOL_LABELS[proto], zorder=3)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Throughput (Kops/sec)')
    ax.set_title(f'Weak Ratio Sweep: Throughput ({rtt_label})\n{workload}', fontsize=11)
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left')
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)

def main():
    base = base_dir()
    dist_csv = os.path.join(base, 'results', 'eval-dist-20260307-w5', 'summary-exp3.2.csv')
    local_csv = os.path.join(base, 'results', 'eval-local-20260307-final3', 'summary-exp3.2.csv')
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    dist_rows = load_csv(dist_csv)
    local_rows = load_csv(local_csv)

    # Latency figure (with P50 and P99)
    fig1, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))
    plot_latency_subplot(ax1, dist_rows, 'RTT = 50 ms', WORKLOAD_DIST)
    plot_latency_subplot(ax2, local_rows, 'RTT = 100 ms', WORKLOAD_LOCAL)
    plt.tight_layout(w_pad=3)
    save_figure(fig1, out_dir, 'exp3.2-t-property-latency')

    # Throughput figure
    fig2, (ax3, ax4) = plt.subplots(1, 2, figsize=(12, 4.5))
    plot_throughput_subplot(ax3, dist_rows, 'RTT = 50 ms', WORKLOAD_DIST)
    plot_throughput_subplot(ax4, local_rows, 'RTT = 100 ms', WORKLOAD_LOCAL)
    plt.tight_layout(w_pad=3)
    save_figure(fig2, out_dir, 'exp3.2-t-property-throughput')

if __name__ == '__main__':
    main()
