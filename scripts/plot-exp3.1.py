#!/usr/bin/env python3
"""
Exp 3.1: CURP-HO vs CURP-HT vs CURP baseline — 2×2 figure.

Layout:
  ┌─────────────────────┬────────────────────────┐
  │ w=5%  tput vs lat   │ w=5%  latency CDF      │
  ├─────────────────────┼────────────────────────┤
  │ w=50% tput vs lat   │ w=50% latency CDF      │
  └─────────────────────┴────────────────────────┘

Data sources:
  - CSV: results/latest/exp3.1.csv
  - CDF: per-run latencies.json (at fixed thread count)
"""

import json
import numpy as np
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

PROTOCOLS     = ['curpho', 'curpht', 'curp-baseline']
HYBRID_PROTOS = {'curpho', 'curpht'}

WRITE_GROUPS = [
    (5,  'Write Ratio 5%'),
    (50, 'Write Ratio 50%'),
]

CDF_THREADS = 32

RESULT_DIRS = {
    5:  ('eval-exp3.1-fix-20260324', 'exp3.1'),
    50: ('eval-exp3.1-fix-20260324', 'exp3.1'),
}


def load_latencies(base, proto, wg, threads):
    """Load per-request latencies from latencies.json."""
    dir_name, sub = RESULT_DIRS.get(wg, ('eval-exp3.1-20260321', 'exp3.1'))
    path = os.path.join(base, 'results', dir_name,
                        sub, f'w{wg}', proto,
                        f't{threads}', 'run1', 'latencies.json')
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)


def plot_tput_lat(ax, rows, wg, wg_label):
    """Left subplot: throughput vs weighted avg latency."""
    for proto in PROTOCOLS:
        filtered = [r for r in rows
                    if r['protocol'] == proto
                    and int(r['write_group']) == wg]
        filtered.sort(key=lambda r: int(r['threads']))
        if not filtered:
            continue
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        throughput = [float(r['avg_throughput']) for r in filtered]
        avg_lat = [float(r['avg_lat']) for r in filtered]

        x, y = clean_pairs(throughput, avg_lat)
        # Drop last point for curpho w50% (outlier)
        if proto == 'curpho' and wg == 50 and len(x) > 1:
            x, y = x[:-1], y[:-1]
        if x:
            ax.plot(x, y, color=color, marker=marker, markersize=8, linewidth=2.5, label=label, zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg Latency (ms)')
    ax.set_title(f'Throughput vs Latency ({wg_label})', fontsize=13)
    ax.legend(loc='upper left', fontsize=10, ncol=1)
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))


def plot_cdf(ax, base, wg, wg_label):
    """Right subplot: combined latency CDF at fixed thread count."""
    for proto in PROTOCOLS:
        lats = load_latencies(base, proto, wg, CDF_THREADS)
        if lats is None:
            continue
        color  = PROTOCOL_COLORS[proto]
        label  = PROTOCOL_LABELS[proto]

        all_lats = (lats.get('strong_write', []) +
                    lats.get('strong_read', []) +
                    lats.get('weak_write', []) +
                    lats.get('weak_read', []))
        if all_lats:
            sorted_lats = np.sort(all_lats)
            cdf = np.arange(1, len(sorted_lats) + 1) / len(sorted_lats)
            ax.plot(sorted_lats, cdf, color=color, linewidth=2.5,
                    label=label, zorder=3)

    ax.set_xlabel('Latency (ms)')
    ax.set_ylabel('CDF')
    ax.set_title(f'Latency CDF, 96 clients ({wg_label})', fontsize=13)
    ax.legend(loc='lower right', fontsize=10, ncol=1)
    ax.set_ylim(0, 1.02)
    ax.set_xlim(left=0, right=150)
    ax.set_xticks([0, 50, 100, 150])


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp3.1.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    fig, axes = plt.subplots(1, 4, figsize=(24, 4))

    labels = ['(a)', '(b)', '(c)', '(d)']
    for col_idx, (wg, wg_label) in enumerate(WRITE_GROUPS):
        plot_tput_lat(axes[col_idx * 2], rows, wg, wg_label)
        plot_cdf(axes[col_idx * 2 + 1], base, wg, wg_label)

    for i, ax in enumerate(axes):
        ax.text(0.5, -0.28, labels[i], transform=ax.transAxes,
                fontsize=14, fontweight='bold', ha='center')

    plt.tight_layout(w_pad=1.5)
    plt.subplots_adjust(bottom=0.18)
    save_figure(fig, out_dir, 'exp3.1-throughput-latency')


if __name__ == '__main__':
    main()
