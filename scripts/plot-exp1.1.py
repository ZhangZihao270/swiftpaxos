#!/usr/bin/env python3
"""
Exp 1.1: Raft-HT vs baselines — 2×2 figure.

Layout (one figure):
  ┌─────────────────────┬────────────────────────┐
  │ w=5%  tput vs lat   │ w=5%  latency CDF      │
  ├─────────────────────┼────────────────────────┤
  │ w=50% tput vs lat   │ w=50% latency CDF      │
  └─────────────────────┴────────────────────────┘

Data sources:
  - CSV: results/latest/exp1.1-4proto.csv + exp1.1-pileusht.csv
  - CDF: per-run latencies.json (at fixed thread count)
"""

import json
import numpy as np
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

# Protocols to plot and their display order
PROTOCOLS = ['raft', 'raftht', 'mongotunable', 'pileus', 'pileusht']

# Protocols that have distinct weak-op latency worth plotting
HYBRID_PROTOS = {'raftht', 'mongotunable', 'pileus', 'pileusht'}

WRITE_GROUPS = [
    (5,  'w=5%'),
    (50, 'w=50%'),
]

# Thread count for CDF subplot
CDF_THREADS = 32

# Map protocol names to result directory names
PROTO_DIR = {
    'raft':          'raft',
    'raftht':        'raftht',
    'mongotunable':  'mongotunable',
    'pileus':        'pileus',
    'pileusht':      'pileusht',
}

# Result directories (4-proto and pileusht are separate runs)
RESULT_DIRS = {
    'raft':         'eval-exp1.1-4proto-20260315',
    'raftht':       'eval-exp1.1-4proto-20260315',
    'mongotunable': 'eval-exp1.1-4proto-20260315',
    'pileus':       'eval-exp1.1-4proto-20260315',
    'pileusht':     'eval-exp1.1-pileusht-v2-20260316',
}


def load_latencies(base, proto, wg, threads):
    """Load per-request latencies from latencies.json."""
    result_dir = RESULT_DIRS.get(proto)
    proto_dir = PROTO_DIR.get(proto, proto)
    path = os.path.join(base, 'results', result_dir,
                        'exp1.1', f'w{wg}', proto_dir,
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

        # Compute combined avg latency: weighted mean of strong and weak p50.
        # For non-hybrid (Raft): weak p50 is N/A, just use strong.
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
    ax.set_title(f'Throughput vs Latency ({wg_label})', fontsize=11)
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
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        # Combine all latencies into one list
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
    # Power scale (x^0.4): compresses low end, stretches high end
    ax.set_xscale('function', functions=(
        lambda x: np.power(np.clip(x, 1e-3, None), 0.4),
        lambda x: np.power(np.clip(x, 1e-3, None), 1/0.4)))
    ax.set_xlim(left=0.1, right=300)
    ax.set_xticks([0.1, 1, 10, 50, 100, 200])
    ax.get_xaxis().set_major_formatter(ticker.ScalarFormatter())


def main():
    base = base_dir()

    # CSV data for throughput-latency subplots
    proto4_csv   = os.path.join(base, 'results', 'latest', 'exp1.1-4proto.csv')
    pileusht_csv = os.path.join(base, 'results', 'latest', 'exp1.1-pileusht.csv')
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = merge_rows(load_csv(proto4_csv), load_csv(pileusht_csv))

    fig, axes = plt.subplots(2, 2, figsize=(12, 9))

    for row_idx, (wg, wg_label) in enumerate(WRITE_GROUPS):
        plot_tput_lat(axes[row_idx, 0], rows, wg, wg_label)
        plot_cdf(axes[row_idx, 1], base, wg, wg_label)

    plt.tight_layout(h_pad=3, w_pad=3)
    save_figure(fig, out_dir, 'exp1.1-throughput-latency')


if __name__ == '__main__':
    main()
