#!/usr/bin/env python3
"""
Exp TAO: CURP-HO vs CURP-HT vs CURP baseline — TAO-like benchmark.

Layout:
  ┌─────────────────────┬────────────────────────┐
  │ tput vs latency     │ latency CDF            │
  └─────────────────────┴────────────────────────┘

Data sources:
  - CSV: results/latest/exp-tao.csv
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

CDF_THREADS = 32

# Default result directory — override with env var or edit after running experiment
RESULT_DIR = os.environ.get('TAO_RESULT_DIR', '')


def load_latencies(base, proto, threads):
    """Load per-request latencies from latencies.json."""
    if RESULT_DIR:
        path = os.path.join(base, 'results', RESULT_DIR,
                            'exp-tao', proto,
                            f't{threads}', 'run1', 'latencies.json')
    else:
        # Try to find any result dir matching eval-exp-tao-*
        results_dir = os.path.join(base, 'results')
        if os.path.isdir(results_dir):
            for d in sorted(os.listdir(results_dir), reverse=True):
                if d.startswith('eval-exp-tao-'):
                    path = os.path.join(results_dir, d,
                                        'exp-tao', proto,
                                        f't{threads}', 'run1', 'latencies.json')
                    if os.path.exists(path):
                        break
            else:
                return None
        else:
            return None
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)


def compute_avg_lat(row):
    """Compute weighted average latency from strong/weak p50 values."""
    s_p50 = get_val(row, 'avg_s_p50')
    w_p50 = get_val(row, 'avg_w_p50')
    if s_p50 is not None and w_p50 is not None:
        # TAO: ~5% strong, ~95% weak
        return 0.05 * s_p50 + 0.95 * w_p50
    elif s_p50 is not None:
        return s_p50
    elif w_p50 is not None:
        return w_p50
    return None


def plot_tput_lat(ax, rows):
    """Left subplot: throughput vs weighted avg latency."""
    for proto in PROTOCOLS:
        filtered = [r for r in rows if r['protocol'] == proto]
        filtered.sort(key=lambda r: int(r['threads']))
        if not filtered:
            continue
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        throughput = [float(r['avg_throughput']) for r in filtered]
        avg_lat = [compute_avg_lat(r) for r in filtered]

        x, y = clean_pairs(throughput, avg_lat)
        if x:
            ax.plot(x, y, color=color, marker=marker, markersize=8,
                    linewidth=2.5, label=label, zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg Latency (ms)')
    ax.set_title('TAO Workload: Throughput vs Latency', fontsize=13)
    ax.legend(loc='upper left', fontsize=10, ncol=1)
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))


def plot_cdf(ax, base):
    """Right subplot: combined latency CDF at fixed thread count."""
    for proto in PROTOCOLS:
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
            ax.plot(sorted_lats, cdf, color=color, linewidth=2.5,
                    label=label, zorder=3)

    ax.set_xlabel('Latency (ms)')
    ax.set_ylabel('CDF')
    ax.set_title(f'Latency CDF, {CDF_THREADS} clients (TAO)', fontsize=13)
    ax.legend(loc='lower right', fontsize=10, ncol=1)
    ax.set_ylim(0, 1.02)
    ax.set_xlim(left=0, right=150)
    ax.set_xticks([0, 50, 100, 150])


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp-tao.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()

    fig, axes = plt.subplots(1, 2, figsize=(12, 4))

    if os.path.exists(csv_path):
        rows = load_csv(csv_path)
        plot_tput_lat(axes[0], rows)
    else:
        print(f'WARNING: CSV not found: {csv_path}')
        axes[0].text(0.5, 0.5, 'No data yet\n(run eval-exp-tao.sh first)',
                     transform=axes[0].transAxes, ha='center', va='center',
                     fontsize=12, color='gray')
        axes[0].set_title('TAO Workload: Throughput vs Latency', fontsize=13)

    plot_cdf(axes[1], base)

    labels = ['(a)', '(b)']
    for i, ax in enumerate(axes):
        ax.text(0.5, -0.28, labels[i], transform=ax.transAxes,
                fontsize=14, fontweight='bold', ha='center')

    plt.tight_layout(w_pad=1.5)
    plt.subplots_adjust(bottom=0.18)
    save_figure(fig, out_dir, 'exp-tao-throughput-latency')


if __name__ == '__main__':
    main()
