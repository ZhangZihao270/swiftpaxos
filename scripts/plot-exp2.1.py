#!/usr/bin/env python3
"""
Exp 2.1: EPaxos-HO vs Vanilla EPaxos — 2×2 figure.

Layout:
  ┌─────────────────────┬────────────────────────┐
  │ w=5%  tput vs lat   │ w=5%  latency CDF      │
  ├─────────────────────┼────────────────────────┤
  │ w=50% tput vs lat   │ w=50% latency CDF      │
  └─────────────────────┴────────────────────────┘

Data sources:
  - CSV: results/latest/exp2.1.csv
  - CDF: per-run latencies.json (at fixed thread count)
"""

import json
import numpy as np
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

PROTOCOLS     = ['epaxos', 'epaxosho']
HYBRID_PROTOS = {'epaxosho'}

WRITE_GROUPS = [
    (5,  'w=5%'),
    (50, 'w=50%'),
]

CDF_THREADS = 32

RESULT_DIR = 'eval-exp2.1-20260314'


def load_latencies(base, proto, wg, threads):
    """Load per-request latencies from latencies.json."""
    path = os.path.join(base, 'results', RESULT_DIR,
                        'exp2.1', f'w{wg}', proto,
                        f't{threads}', 'run1', 'latencies.json')
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)


def plot_tput_lat(ax, rows, wg, wg_label):
    """Left subplot: throughput vs average latency (combined strong+weak)."""
    for proto in PROTOCOLS:
        data = extract_tput_latency_wg(rows, proto, wg)
        if not data['throughput']:
            continue
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        avg_lat = []
        for s, w in zip(data['s_p50'], data['w_p50']):
            if s is None:
                avg_lat.append(None)
            elif w is None or proto not in HYBRID_PROTOS:
                avg_lat.append(s)
            else:
                avg_lat.append((s + w) / 2.0)

        x, y = clean_pairs(data['throughput'], avg_lat)
        x, y = pareto_frontier(x, y)
        if x:
            ax.plot(x, y, color=color, marker=marker, label=label, zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg P50 Latency (ms)')
    ax.set_title(f'EPaxos-HO vs EPaxos ({wg_label})', fontsize=11)
    ax.legend(loc='upper left', fontsize=8, ncol=1)
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
            ax.plot(sorted_lats, cdf, color=color, linewidth=1.8,
                    label=label, zorder=3)

    ax.set_xlabel('Latency (ms)')
    ax.set_ylabel('CDF')
    ax.set_title(f'Latency CDF at t={CDF_THREADS} ({wg_label})', fontsize=11)
    ax.legend(loc='lower right', fontsize=8, ncol=1)
    ax.set_ylim(0, 1.02)
    ax.set_xscale('function', functions=(
        lambda x: np.power(np.clip(x, 1e-3, None), 0.4),
        lambda x: np.power(np.clip(x, 1e-3, None), 1/0.4)))
    ax.set_xlim(left=0.1, right=300)
    ax.set_xticks([0.1, 1, 10, 50, 100, 200])
    ax.get_xaxis().set_major_formatter(ticker.ScalarFormatter())


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp2.1.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    fig, axes = plt.subplots(2, 2, figsize=(12, 9))

    for row_idx, (wg, wg_label) in enumerate(WRITE_GROUPS):
        plot_tput_lat(axes[row_idx, 0], rows, wg, wg_label)
        plot_cdf(axes[row_idx, 1], base, wg, wg_label)

    plt.tight_layout(h_pad=3, w_pad=3)
    save_figure(fig, out_dir, 'exp2.1-throughput-latency')


if __name__ == '__main__':
    main()
