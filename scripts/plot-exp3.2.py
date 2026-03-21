#!/usr/bin/env python3
"""
Exp 3.2: T Property Verification — Strong Latency Stability.

Layout (1×2):
  ┌───────────────────────────┬───────────────────────────┐
  │ Strong Latency vs WR      │ Throughput vs WR          │
  │ (p50 solid + p99 dashed)  │                           │
  └───────────────────────────┴───────────────────────────┘

Fixed: t=32, zipfSkew=0.99, w=50%.
Sweep weak ratio: 0%, 25%, 50%, 75%, 99%.
p50/p99 computed from raw latencies.json.
"""

import json
import numpy as np
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

PROTOCOLS  = ['curpht', 'curpho']
WEAK_RATIOS = [0, 25, 50, 75, 99]

RESULT_DIR = 'eval-exp3.2-v4'


def load_latency_percentiles(base, proto, wr):
    """Compute strong p50/p99 from raw latencies.json."""
    path = os.path.join(base, 'results', RESULT_DIR,
                        'exp3.2', proto, f'wr{wr}',
                        'run1', 'latencies.json')
    if not os.path.exists(path):
        return None
    with open(path) as f:
        d = json.load(f)
    s_lats = np.array(d.get('strong_write', []) + d.get('strong_read', []))
    if len(s_lats) == 0:
        return None
    return {
        's_p50': np.median(s_lats),
        's_p99': np.percentile(s_lats, 99),
    }


def collect_latencies(base):
    """Collect p50/p99 for all protocols and weak ratios."""
    data = {}
    for proto in PROTOCOLS:
        data[proto] = {'wr': [], 's_p50': [], 's_p99': []}
        for wr in WEAK_RATIOS:
            percs = load_latency_percentiles(base, proto, wr)
            if percs is None:
                continue
            data[proto]['wr'].append(wr)
            data[proto]['s_p50'].append(percs['s_p50'])
            data[proto]['s_p99'].append(percs['s_p99'])
    return data


def extract_throughput(rows, protocol):
    """Extract throughput from CSV sorted by weak_ratio."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['weak_ratio']))

    def _get(r, *keys):
        for k in keys:
            v = r.get(k)
            if v is not None and v != '' and v != 'N/A':
                try:
                    return float(v)
                except ValueError:
                    pass
        return None

    return {
        'wr':         [int(r['weak_ratio']) for r in filtered],
        'throughput': [_get(r, 'avg_throughput', 'throughput') for r in filtered],
    }


def plot_latency(ax, lat_data):
    """Left subplot: strong p50 + p99 vs weak ratio."""
    for proto in PROTOCOLS:
        d = lat_data[proto]
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        # p50 — solid
        ax.plot(d['wr'], d['s_p50'], color=color, marker=marker,
                label=f'{label} (p50)', zorder=3)
        # p99 — dashed
        ax.plot(d['wr'], d['s_p99'], color=color, marker=marker,
                markersize=5, linestyle='--', alpha=0.7,
                label=f'{label} (p99)', zorder=2)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Strong Latency (ms)')
    ax.set_title('T Property: Strong Latency vs Weak Ratio', fontsize=11)
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left', fontsize=8, ncol=1)
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)


def plot_throughput(ax, rows):
    """Right subplot: throughput vs weak ratio."""
    for proto in PROTOCOLS:
        data = extract_throughput(rows, proto)
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        ax.plot(data['wr'],
                [t / 1000 if t else None for t in data['throughput']],
                color=color, marker=marker, label=label, zorder=3)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Throughput (Kops/sec)')
    ax.set_title('Throughput vs Weak Ratio', fontsize=11)
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left', fontsize=8)
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp3.2.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)
    lat_data = collect_latencies(base)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))
    plot_latency(ax1, lat_data)
    plot_throughput(ax2, rows)
    plt.tight_layout(w_pad=3)
    save_figure(fig, out_dir, 'exp3.2-t-property')


if __name__ == '__main__':
    main()
