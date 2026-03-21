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

SKEWS = [0, 0.25, 0.5, 0.75, 0.99, 1.2, 1.5, 2.0]

RESULT_DIR = 'eval-exp2.2-20260314'


def extract_skew_series(rows, protocol):
    """Extract data sorted by zipf_skew for a given protocol."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: float(r['zipf_skew']))
    return {
        'skew':       [float(r['zipf_skew']) for r in filtered],
        'throughput': [float(r['avg_throughput']) for r in filtered],
    }


def load_latency_percentiles(base, proto, skew):
    """Compute p50 and p99 from raw latencies.json."""
    skew_dir = f'z{skew}' if skew != int(skew) else f'z{int(skew)}'
    # Try both formats
    for sd in [f'z{skew}', f'z{int(skew)}' if skew == int(skew) else None]:
        if sd is None:
            continue
        path = os.path.join(base, 'results', RESULT_DIR,
                            'exp2.2', proto, sd,
                            'run1', 'latencies.json')
        if os.path.exists(path):
            with open(path) as f:
                d = json.load(f)
            s_lats = np.array(d.get('strong_write', []) + d.get('strong_read', []))
            w_lats = np.array(d.get('weak_write', []) + d.get('weak_read', []))
            result = {}
            if len(s_lats):
                result['s_p50'] = np.median(s_lats)
                result['s_p99'] = np.percentile(s_lats, 99)
            if len(w_lats):
                result['w_p50'] = np.median(w_lats)
                result['w_p99'] = np.percentile(w_lats, 99)
            return result
    return None


def collect_latencies(base):
    """Collect p50/p99 for all protocols and skews."""
    data = {}
    for proto in PROTOCOLS:
        data[proto] = {'skew': [], 's_p50': [], 's_p99': [],
                       'w_p50': [], 'w_p99': []}
        for skew in SKEWS:
            percs = load_latency_percentiles(base, proto, skew)
            if percs is None:
                continue
            data[proto]['skew'].append(skew)
            data[proto]['s_p50'].append(percs.get('s_p50'))
            data[proto]['s_p99'].append(percs.get('s_p99'))
            data[proto]['w_p50'].append(percs.get('w_p50'))
            data[proto]['w_p99'].append(percs.get('w_p99'))
    return data


def plot_throughput(ax, rows):
    """Left subplot: throughput vs Zipf skew."""
    for proto in PROTOCOLS:
        data = extract_skew_series(rows, proto)
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        ax.plot(data['skew'], [t / 1000 for t in data['throughput']],
                color=color, marker=marker, label=label, zorder=3)

    ax.set_xlabel('Zipf Skew')
    ax.set_ylabel('Throughput (Kops/sec)')
    ax.set_title('Throughput vs Conflict Rate', fontsize=11)
    ax.legend(loc='upper right', fontsize=8)
    ax.set_xlim(-0.1, 2.1)
    ax.set_ylim(bottom=0)


def plot_latency_broken(fig, gs_slot, lat_data):
    """Right subplot: broken y-axis p50 — strong (top) and weak (bottom)."""
    inner = gs_slot.subgridspec(2, 1, height_ratios=[3, 1], hspace=0.12)
    ax_top = fig.add_subplot(inner[0])
    ax_bot = fig.add_subplot(inner[1])

    # --- Top: strong p50 (50-130ms range) ---
    for proto in PROTOCOLS:
        d = lat_data[proto]
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        ax_top.plot(d['skew'], d['s_p50'], color=color, marker=marker,
                    label=f'{label} (strong)', zorder=3)

    ax_top.set_ylabel('Strong P50 Latency (ms)')
    ax_top.set_title('P50 Latency vs Conflict Rate', fontsize=11)
    ax_top.legend(loc='upper left', fontsize=8)
    ax_top.set_xlim(-0.1, 2.1)
    ax_top.set_ylim(50, 130)
    ax_top.tick_params(labelbottom=False)

    # --- Bottom: weak p50 (0-1.5ms range) ---
    for proto in PROTOCOLS:
        d = lat_data[proto]
        color  = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label  = PROTOCOL_LABELS[proto]

        if proto in HYBRID_PROTOS and d['w_p50'][0] is not None:
            ax_bot.plot(d['skew'], d['w_p50'], color=color, marker=marker,
                        label=f'{label} (weak)', zorder=3)

    ax_bot.set_xlabel('Zipf Skew')
    ax_bot.set_ylabel('Weak P50 (ms)')
    ax_bot.legend(loc='upper left', fontsize=8)
    ax_bot.set_xlim(-0.1, 2.1)
    ax_bot.set_ylim(0, 1.5)

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


def main():
    base = base_dir()
    csv_path = os.path.join(base, 'results', 'latest', 'exp2.2.csv')
    out_dir  = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_csv(csv_path)
    lat_data = collect_latencies(base)

    import matplotlib.gridspec as gridspec
    fig = plt.figure(figsize=(12, 5))
    gs = gridspec.GridSpec(1, 2, figure=fig, wspace=0.3)

    # Left: throughput
    ax_tput = fig.add_subplot(gs[0])
    plot_throughput(ax_tput, rows)

    # Right: broken y-axis latency
    plot_latency_broken(fig, gs[1], lat_data)

    save_figure(fig, out_dir, 'exp2.2-conflict-sweep')


if __name__ == '__main__':
    main()
