#!/usr/bin/env python3
"""
Exp TAO: TAO-like benchmark — 1×2 figure.

Layout:
  ┌─────────────────────┬────────────────────────┐
  │ tput vs latency     │ latency CDF (t=8)      │
  └─────────────────────┴────────────────────────┘

Data sources:
  - CSV: results/latest/exp-tao.csv
  - CDF: per-run latencies.json
"""

import json
import numpy as np
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

ALL_PROTOS = ['raft-baseline', 'raftht',
              'epaxos-baseline', 'epaxosho',
              'curp-baseline', 'curpht', 'curpho',
              'mongotunable',
              'pileus', 'pileusht']

CDF_THREADS = 8

# Result directory for CDF latencies.json
RESULT_DIR = 'eval-exp-tao-20260330'


def load_latencies(base, proto, threads):
    """Load per-request latencies from latencies.json."""
    path = os.path.join(base, 'results', RESULT_DIR,
                        'exp-tao', proto,
                        f't{threads}', 'run1', 'latencies.json')
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)


def plot_tput_lat(ax, rows, protocols, title):
    """Throughput vs weighted avg latency."""
    for proto in protocols:
        filtered = [r for r in rows if r['protocol'] == proto]
        filtered.sort(key=lambda r: int(r['threads']))
        if not filtered:
            continue
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        throughput = [float(r['avg_throughput']) for r in filtered]
        avg_lat = [float(r['avg_lat']) for r in filtered]

        x, y = clean_pairs(throughput, avg_lat)
        top_protos = {'raftht', 'epaxosho', 'curpho', 'curpht'}
        z = 5 if proto in top_protos else 3
        if x:
            ax.plot(x, y, color=color, marker=marker, markersize=8,
                    linewidth=2.5, label=label, zorder=z)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg Latency (ms)')
    ax.set_title(title, fontsize=13)
    ax.legend(loc='upper right', fontsize=10, ncol=1)
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))


def plot_cdf(ax, base, protocols, title):
    """Combined latency CDF at fixed thread count."""
    for proto in protocols:
        lats = load_latencies(base, proto, CDF_THREADS)
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
            top_protos = {'raftht', 'epaxosho', 'curpho', 'curpht'}
            z = 5 if proto in top_protos else 3
            ax.plot(sorted_lats, cdf, color=color, linewidth=2.5,
                    label=label, zorder=z)

    ax.set_xlabel('Latency (ms)')
    ax.set_ylabel('CDF')
    ax.set_title(title, fontsize=13)
    ax.legend(loc='lower right', fontsize=10, ncol=1)
    ax.set_ylim(0, 1.02)
    ax.set_xlim(left=0, right=150)
    ax.set_xticks([0, 50, 100, 150])


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp-tao.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    fig, axes = plt.subplots(1, 2, figsize=(12, 4))

    plot_tput_lat(axes[0], rows, ALL_PROTOS, 'TAO: Throughput vs Latency')
    plot_cdf(axes[1], base, ALL_PROTOS, f'TAO: Latency CDF ({CDF_THREADS * 6} clients)')

    labels = ['(a)', '(b)']
    for i, ax in enumerate(axes):
        ax.text(0.5, -0.28, labels[i], transform=ax.transAxes,
                fontsize=14, fontweight='bold', ha='center')

    plt.tight_layout(w_pad=1.5)
    plt.subplots_adjust(bottom=0.18)
    save_figure(fig, out_dir, 'exp-tao-throughput-latency')


if __name__ == '__main__':
    main()
