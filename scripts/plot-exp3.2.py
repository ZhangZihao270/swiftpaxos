#!/usr/bin/env python3
"""
Exp 3.2: T Property Verification — Linear Latency Stability.

Layout (1×2):
  ┌───────────────────────────┬───────────────────────────┐
  │ Linear Latency vs WR      │ Throughput vs WR          │
  │ (p50 solid + p95 dashed)  │                           │
  └───────────────────────────┴───────────────────────────┘

Fixed: t=32, zipfSkew=0.99, w=50%.
Sweep weak ratio: 0%, 25%, 50%, 75%, 99%.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

PROTOCOLS = ['curpht', 'curpho']


def _get(r, *keys):
    for k in keys:
        v = r.get(k)
        if v is not None and v != '' and v != 'N/A':
            try:
                return float(v)
            except ValueError:
                pass
    return None


def extract_data(rows, protocol):
    """Extract throughput and latency from CSV sorted by weak_ratio."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['weak_ratio']))
    return {
        'wr':         [int(r['weak_ratio']) for r in filtered],
        'throughput': [_get(r, 'avg_throughput', 'throughput') for r in filtered],
        's_p50':     [_get(r, 'avg_s_p50') for r in filtered],
        's_p95':     [_get(r, 'avg_s_p99') for r in filtered],
    }


def plot_latency(ax, rows):
    """Left subplot: strong p50 + p95 vs weak ratio."""
    for proto in PROTOCOLS:
        d = extract_data(rows, proto)
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        # p50 — solid
        ax.plot(d['wr'], d['s_p50'], color=color, marker=marker, markersize=8, linewidth=2.5,
                label=f'{label} (p50)', zorder=3)
        # p99 — dashed
        ax.plot(d['wr'], d['s_p95'], color=color, marker=marker,
                markersize=6, linewidth=2, linestyle='--', alpha=0.7,
                label=f'{label} (p99)', zorder=2)

    ax.set_xlabel('Causal Operation Ratio (%)')
    ax.set_ylabel('Linear Latency\n(ms)')
    
    ax.set_xticks([0, 25, 50, 75, 99])
    ax.legend(loc='upper left', fontsize=10, ncol=1)
    ax.set_xlim(-5, 105)
    ax.set_ylim(top=120)


def plot_throughput(ax, rows):
    """Right subplot: throughput vs weak ratio."""
    for proto in PROTOCOLS:
        d = extract_data(rows, proto)
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        ax.plot(d['wr'],
                [t / 1000 if t else None for t in d['throughput']],
                color=color, marker=marker, markersize=8, linewidth=2.5, label=label, zorder=3)

    ax.set_xlabel('Causal Operation Ratio (%)')
    ax.set_ylabel('Throughput\n(Kops/sec)')
    
    ax.set_xticks([0, 25, 50, 75, 99])
    ax.legend(loc='upper left', fontsize=10)
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp3.2.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 3))
    plot_throughput(ax1, rows)
    plot_latency(ax2, rows)

    ax1.text(0.5, -0.40, '(a)', transform=ax1.transAxes,
             fontsize=14, fontweight='bold', ha='center')
    ax2.text(0.5, -0.40, '(b)', transform=ax2.transAxes,
             fontsize=14, fontweight='bold', ha='center')

    plt.tight_layout(w_pad=3)
    plt.subplots_adjust(bottom=0.28)
    save_figure(fig, out_dir, 'exp3.2-t-property')


if __name__ == '__main__':
    main()
