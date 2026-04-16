#!/usr/bin/env python3
"""
Exp 2.2: Conflict Rate Sweep — EPaxos-HO vs EPaxos.

Layout (1×2):
  ┌───────────────────────┬───────────────────────┐
  │ Throughput vs Skew    │ Latency vs Skew       │
  │                       │ (broken y-axis,       │
  │                       │  p50 + p99)           │
  └───────────────────────┴───────────────────────┘

Fixed: t=32, w=50%, sweep Zipf skew.
Latency subplot uses broken y-axis: top for strong (50-250ms), bottom for weak (0-2ms).
"""

import json
import numpy as np
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

PROTOCOLS     = ['epaxos', 'epaxosho']
HYBRID_PROTOS = {'epaxosho'}

SKEWS = [0, 0.25, 0.5, 0.75, 0.99]

RESULT_DIR = 'eval-exp2.2-fix-20260324'


def extract_skew_series(rows, protocol):
    """Extract data sorted by zipf_skew for a given protocol."""
    filtered = [r for r in rows if r['protocol'] == protocol and float(r['zipf_skew']) in SKEWS]
    filtered.sort(key=lambda r: float(r['zipf_skew']))
    return {
        'skew':       [float(r['zipf_skew']) for r in filtered],
        'throughput': [float(r['avg_throughput']) for r in filtered],
    }


def collect_latencies(rows):
    """Collect p50/p99 for all protocols and skews from CSV."""
    data = {}
    for proto in PROTOCOLS:
        data[proto] = {'skew': [], 's_p50': [], 's_p99': [],
                       'w_p50': [], 'w_p99': []}
        filtered = [r for r in rows if r['protocol'] == proto and float(r['zipf_skew']) in SKEWS]
        filtered.sort(key=lambda r: float(r['zipf_skew']))
        for r in filtered:
            data[proto]['skew'].append(float(r['zipf_skew']))
            data[proto]['s_p50'].append(float(r['avg_s_p50']) if float(r['avg_s_p50']) > 0 else None)
            data[proto]['s_p99'].append(float(r['avg_s_p99']) if float(r['avg_s_p99']) > 0 else None)
            data[proto]['w_p50'].append(float(r['avg_w_p50']) if float(r['avg_w_p50']) > 0 else None)
            data[proto]['w_p99'].append(float(r['avg_w_p99']) if float(r['avg_w_p99']) > 0 else None)
    return data


def plot_throughput(ax, rows):
    """Left subplot: throughput vs Zipf skew."""
    for proto in PROTOCOLS:
        data = extract_skew_series(rows, proto)
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        ax.plot(data['skew'], [t / 1000 for t in data['throughput']],
                color=color, marker=marker, markersize=11, label=label, zorder=3)

    ax.set_xlabel('Zipf Skew')
    ax.set_ylabel('Throughput\n(Kops/sec)')

    ax.legend(loc='lower left')
    ax.set_xlim(-0.05, 1.04)
    ax.set_xticks([0, 0.25, 0.5, 0.75, 0.99])
    ax.set_ylim(bottom=0)


def plot_latency_broken(fig, gs_slot, lat_data):
    """Right subplot: broken y-axis p50 — strong (top) and weak (bottom)."""
    inner = gs_slot.subgridspec(2, 1, height_ratios=[2.5, 1], hspace=0.12)
    ax_top = fig.add_subplot(inner[0])
    ax_bot = fig.add_subplot(inner[1])

    # Distinct markers per protocol and metric
    p50_markers = {'epaxos': 'o', 'epaxosho': 'D'}
    p99_markers = {'epaxos': 's', 'epaxosho': '^'}

    # --- Top: strong p50 + p99 ---
    for proto in PROTOCOLS:
        d = lat_data[proto]
        color  = PROTOCOL_COLORS[proto]
        label  = PROTOCOL_LABELS[proto]

        ax_top.plot(d['skew'], d['s_p50'], color=color, marker=p50_markers[proto],
                    markersize=11, linewidth=2.5,
                    label=f'{label} linear (p50)', zorder=3)
        ax_top.plot(d['skew'], d['s_p99'], color=color, marker=p99_markers[proto],
                    markersize=10, linewidth=2, linestyle='--', alpha=0.7,
                    fillstyle='none', markeredgewidth=2,
                    label=f'{label} linear (p99)', zorder=2)

    ax_top.set_ylabel('Latency (ms)', y=0.3)

    ax_top.legend_.remove() if ax_top.get_legend() else None
    ax_top.set_xlim(-0.05, 1.04)
    ax_top.set_xticks([0, 0.25, 0.5, 0.75, 0.99])
    # Auto-scale based on data range
    all_s = [v for proto in PROTOCOLS
             for v in lat_data[proto]['s_p50'] + lat_data[proto]['s_p99']
             if v is not None]
    all_s = [v for proto in PROTOCOLS
             for v in lat_data[proto]['s_p50'] + lat_data[proto]['s_p99']
             if v is not None]
    if all_s:
        ymin = max(0, min(all_s) - 10)
        ax_top.set_ylim(ymin, 250)
    ax_top.set_yticks([50, 100, 150, 200, 250])
    ax_top.tick_params(labelbottom=False)

    # --- Bottom: weak p50 + p99 ---
    for proto in PROTOCOLS:
        d = lat_data[proto]
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        if proto in HYBRID_PROTOS and d['w_p50'][0] is not None:
            causal_color = '#009E73'  # green, distinct from red linear
            ax_bot.plot(d['skew'], d['w_p50'], color=causal_color, marker=p50_markers[proto],
                        markersize=11, linewidth=2.5,
                        label=f'{label} causal (p50)', zorder=3)
            ax_bot.plot(d['skew'], d['w_p99'], color=causal_color, marker=p99_markers[proto],
                        markersize=10, linewidth=2, linestyle='--', alpha=0.7,
                        fillstyle='none', markeredgewidth=2,
                        label=f'{label} causal (p99)', zorder=2)

    ax_bot.set_xlabel('Zipf Skew')
    ax_bot.set_ylabel('')
    ax_bot.legend_.remove() if ax_bot.get_legend() else None
    ax_bot.set_xlim(-0.05, 1.04)
    ax_bot.set_xticks([0, 0.25, 0.5, 0.75, 0.99])
    all_w = [v for proto in PROTOCOLS
             for v in lat_data[proto]['w_p99']
             if v is not None]
    ax_bot.set_ylim(0, max(all_w) + 5 if all_w else 1.5)

    # Break markers
    d_size = 0.015
    kwargs = dict(transform=ax_top.transAxes, color='k', clip_on=False, linewidth=1)
    ax_top.plot((-d_size, d_size), (-d_size, d_size), **kwargs)
    ax_top.plot((1 - d_size, 1 + d_size), (-d_size, d_size), **kwargs)
    kwargs['transform'] = ax_bot.transAxes
    ax_bot.plot((-d_size, d_size), (1 - d_size, 1 + d_size), **kwargs)
    ax_bot.plot((1 - d_size, 1 + d_size), (1 - d_size, 1 + d_size), **kwargs)

    ax_top.spines['bottom'].set_visible(False)
    ax_bot.spines['top'].set_visible(False)

    # Collect all handles and place a single 2-column legend inside top panel (upper left)
    h1, l1 = ax_top.get_legend_handles_labels()
    h2, l2 = ax_bot.get_legend_handles_labels()
    # Interleave: linear p50/p99 then causal p50/p99 — 2 columns x 3 rows
    ax_top.legend(h1 + h2, l1 + l2, loc='upper left',
                  fontsize=12, ncol=2, framealpha=0.9)


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp2.2.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)
    lat_data = collect_latencies(rows)

    import matplotlib.gridspec as gridspec
    fig = plt.figure(figsize=(14, 4.5))
    gs = gridspec.GridSpec(1, 2, figure=fig, wspace=0.25, width_ratios=[1, 2])

    # Left: throughput
    ax_tput = fig.add_subplot(gs[0])
    plot_throughput(ax_tput, rows)

    # Right: broken y-axis latency
    plot_latency_broken(fig, gs[1], lat_data)

    ax_tput.text(0.5, -0.38, '(a) Throughput vs Contention', transform=ax_tput.transAxes,
                 fontsize=22, fontweight='bold', ha='center')
    fig.text(0.73, -0.01, '(b) Latency vs Contention', fontsize=22, fontweight='bold', ha='center')

    plt.subplots_adjust(bottom=0.2)
    save_figure(fig, out_dir, 'exp2.2-conflict-sweep')


if __name__ == '__main__':
    main()
