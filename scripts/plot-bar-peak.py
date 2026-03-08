#!/usr/bin/env python3
"""
Peak Throughput Bar Chart — Distributed Results
Grouped bar chart showing peak throughput for each protocol,
with separate bars for strong-only and hybrid workloads where applicable.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *
import numpy as np

def get_peak_throughput(rows, protocol):
    """Get peak throughput and the latency at that point."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    if not filtered:
        return None
    best = max(filtered, key=lambda r: float(r['throughput']))
    return {
        'throughput': float(best['throughput']),
        'threads': int(best['threads']),
        's_p50': get_val(best, 's_p50'),
        'w_p50': get_val(best, 'w_p50'),
    }

def main():
    base = base_dir()
    exp11_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp1.1.csv')
    exp31_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.1.csv')
    epaxos_csv = os.path.join(base, 'results', 'eval-dist-20260307-w5', 'summary-epaxos.csv')
    out_dir = os.path.join(base, 'plots')

    setup_style()
    exp11_rows = load_csv(exp11_csv)
    exp31_rows = load_csv(exp31_csv)
    epaxos_rows = load_csv_optional(epaxos_csv)

    # Collect peak throughputs
    protocols = [
        ('CURP-HO',       'curpho',        exp31_rows),
        ('CURP-HT',       'curpht',        exp31_rows),
        ('Raft-HT',       'raftht',        exp11_rows),
        ('CURP\n(baseline)', 'curp-baseline', exp31_rows),
        ('EPaxos',        'epaxos',         epaxos_rows),
        ('Raft',          'raft',           exp11_rows),
    ]

    names = []
    peaks = []
    colors = []
    for label, proto, rows in protocols:
        data = get_peak_throughput(rows, proto)
        if data is None:
            continue
        names.append(label)
        peaks.append(data['throughput'] / 1000)  # Kops/sec
        colors.append(PROTOCOL_COLORS[proto])

    fig, ax = plt.subplots(figsize=(7, 4.5))

    x = np.arange(len(names))
    bars = ax.bar(x, peaks, width=0.6, color=colors, edgecolor='white', linewidth=0.5, zorder=3)

    # Add value labels on bars
    for bar, peak in zip(bars, peaks):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 1,
                f'{peak:.0f}K', ha='center', va='bottom', fontsize=10, fontweight='bold')

    ax.set_xticks(x)
    ax.set_xticklabels(names, fontsize=11)
    ax.set_ylabel('Peak Throughput (Kops/sec)')
    ax.set_title('Peak Throughput Comparison (RTT = 50 ms)\n95/5 R/W, 50% weak, Zipfian', fontsize=11)
    ax.set_ylim(0, max(peaks) * 1.15)
    ax.grid(axis='y', alpha=0.3, linestyle='--')

    # Add a horizontal line showing hybrid speedup over baseline
    baseline_peak = peaks[3]  # CURP baseline
    ax.axhline(y=baseline_peak, color='gray', linestyle=':', alpha=0.5, zorder=1)
    ax.text(len(names) - 0.5, baseline_peak + 0.5, 'baseline', fontsize=8, color='gray',
            ha='right', va='bottom')

    plt.tight_layout()
    save_figure(fig, out_dir, 'bar-peak-throughput')

if __name__ == '__main__':
    main()
