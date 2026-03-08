#!/usr/bin/env python3
"""
Exp 3.2 (5-Replica): T Property Verification — Strong Latency Stability
Two figures: strong latency vs weak ratio, and throughput vs weak ratio.
Single-panel each for 5-replica distributed results (RTT=50ms).
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, t=8, Zipfian, 5 replicas'

def extract_weak_ratio_series(rows, protocol):
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['weak_ratio']))
    return {
        'weak_ratio': [int(r['weak_ratio']) for r in filtered],
        's_p50': [float(r['s_p50']) for r in filtered],
        's_p99': [float(r['s_p99']) for r in filtered],
        'throughput': [float(r['throughput']) for r in filtered],
    }

def plot_latency(ax, rows):
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
    ax.set_title(f'T Property: Strong Latency (5 replicas, RTT = 50 ms)\n{WORKLOAD}', fontsize=11)
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left', fontsize=7.5, ncol=2)
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)

def plot_throughput(ax, rows):
    for proto in ['raftht', 'curpht', 'curpho']:
        data = extract_weak_ratio_series(rows, proto)
        ax.plot(data['weak_ratio'], [t/1000 for t in data['throughput']],
                color=PROTOCOL_COLORS[proto], marker=PROTOCOL_MARKERS[proto],
                label=PROTOCOL_LABELS[proto], zorder=3)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Throughput (Kops/sec)')
    ax.set_title(f'Weak Ratio Sweep: Throughput (5 replicas, RTT = 50 ms)\n{WORKLOAD}', fontsize=11)
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left')
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)

def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'eval-5r-20260308', 'summary-exp3.2.csv')
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    # Latency figure
    fig1, ax1 = plt.subplots(1, 1, figsize=(7, 4.5))
    plot_latency(ax1, rows)
    plt.tight_layout()
    save_figure(fig1, out_dir, 'exp3.2-5r-t-property-latency')

    # Throughput figure
    fig2, ax2 = plt.subplots(1, 1, figsize=(7, 4.5))
    plot_throughput(ax2, rows)
    plt.tight_layout()
    save_figure(fig2, out_dir, 'exp3.2-5r-t-property-throughput')

if __name__ == '__main__':
    main()
