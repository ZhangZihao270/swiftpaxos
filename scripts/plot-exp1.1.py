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
    (5,  'Write Ratio 5%'),
    (50, 'Write Ratio 50%'),
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

# Result directories for raw latencies.json
# TODO: after re-running with merged script, all will be in the same dir.
# For now, pileusht was run separately.
RESULT_DIRS = {
    'raft':         'eval-exp1.1-20260321',
    'raftht':       'eval-exp1.1-20260321',
    'mongotunable': 'eval-exp1.1-20260321',
    'pileus':       'eval-exp1.1-20260321',
    'pileusht':     'eval-exp1.1-20260321',
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
        if x:
            zo = 10 if proto == 'raftht' else 3
            ax.plot(x, y, color=color, marker=marker, markersize=8, linewidth=2.5, label=label, zorder=zo)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg Latency (ms)')
    leg_fs = 12 if wg == 5 else None  # smaller legend for (a) to avoid overlap
    ax.legend(loc='upper left', ncol=1, **(dict(fontsize=leg_fs) if leg_fs else {}))
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))


def plot_cdf(ax, base, wg, wg_label):
    """Right subplot: combined latency CDF at fixed thread count."""
    # Draw raftht last (highest zorder) so it appears on top when overlapping
    CDF_ZORDER = {'raftht': 10, 'pileusht': 7, 'mongotunable': 6, 'pileus': 5, 'raft': 4}
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
            lw = 3.0 if proto == 'raftht' else 2.5
            ax.plot(sorted_lats, cdf, color=color, linewidth=lw,
                    label=label, zorder=CDF_ZORDER.get(proto, 3))

    ax.set_xlabel('Latency (ms)')
    ax.set_ylabel('CDF')
    ax.legend(loc='lower right', ncol=1)
    ax.set_ylim(0, 1.02)
    ax.set_xlim(left=0, right=300)
    ax.set_xticks([0, 50, 100, 150, 200, 250, 300])


def main():
    base = base_dir()

    csv_path = os.path.join(base, 'results', 'latest', 'exp1.1.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    fig, axes = plt.subplots(1, 4, figsize=(24, 3.5))

    for col_idx, (wg, wg_label) in enumerate(WRITE_GROUPS):
        plot_tput_lat(axes[col_idx * 2], rows, wg, wg_label)
        plot_cdf(axes[col_idx * 2 + 1], base, wg, wg_label)

    for col_idx, (wg, wg_label) in enumerate(WRITE_GROUPS):
        axes[col_idx * 2].set_title(wg_label)
        axes[col_idx * 2 + 1].set_title(wg_label)

    subcaptions = ['(a) Throughput vs Latency', '(b) Latency CDF',
                   '(c) Throughput vs Latency', '(d) Latency CDF']
    for i, ax in enumerate(axes):
        ax.text(0.5, -0.38, subcaptions[i], transform=ax.transAxes,
                fontsize=16, fontweight='bold', ha='center')

    plt.tight_layout(w_pad=0.8)
    plt.subplots_adjust(bottom=0.22)
    save_figure(fig, out_dir, 'exp1.1-throughput-latency')


if __name__ == '__main__':
    main()
