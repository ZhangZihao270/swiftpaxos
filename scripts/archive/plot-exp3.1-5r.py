#!/usr/bin/env python3
"""
Exp 3.1 (5-Replica): CURP-HO vs CURP-HT vs Baseline — Two figures:
  Fig A: Throughput vs Weighted-Average Latency (single line per protocol)
  Fig B: Latency Breakdown at 3 concurrency levels (bars=P50, lines=P99)
Uses Phase 78 data: median of 3 runs with min/max error bands.
"""

import os
import sys
import numpy as np
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, 50% weak, Zipfian, 5 replicas'
SELECTED_THREADS = [8, 32, 64]


def get_val_from_rows(rows, protocol, threads, key):
    """Get a specific value from aggregated rows."""
    for r in rows:
        if r['protocol'] == protocol and int(r['threads']) == threads:
            return get_val(r, key)
    return None


def plot_tput_vs_avg(ax, rows):
    """Figure A: Throughput vs weighted-average latency."""
    for proto in ['curpho', 'curpht', 'curp-baseline']:
        data = extract_tput_latency_with_errbars(rows, proto)
        color = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label = PROTOCOL_LABELS[proto]

        xs, ys, x_los, x_his = [], [], [], []
        for i in range(len(data['throughput'])):
            xv = data['throughput'][i]
            t = data['threads'][i]
            s_avg = get_val_from_rows(rows, proto, t, 's_avg')
            w_avg = get_val_from_rows(rows, proto, t, 'w_avg')

            if proto == 'curp-baseline':
                yv = s_avg
            else:
                # Weighted average: 50% strong + 50% weak
                if s_avg is not None and w_avg is not None:
                    yv = 0.5 * s_avg + 0.5 * w_avg
                else:
                    yv = s_avg

            if xv is not None and yv is not None and xv > 0:
                xs.append(xv)
                ys.append(yv)
                xlo = (data['throughput_lo'] or [])[i] if data.get('throughput_lo') else None
                xhi = (data['throughput_hi'] or [])[i] if data.get('throughput_hi') else None
                x_los.append(xlo if xlo is not None else xv)
                x_his.append(xhi if xhi is not None else xv)

        if xs:
            peak_idx = max(range(len(xs)), key=lambda j: xs[j])
            xs = xs[:peak_idx + 1]
            ys = ys[:peak_idx + 1]
            x_los = x_los[:peak_idx + 1]
            x_his = x_his[:peak_idx + 1]

            xerr_lo = [x - lo for x, lo in zip(xs, x_los)]
            xerr_hi = [hi - x for x, hi in zip(xs, x_his)]

            ax.errorbar(xs, ys, xerr=[xerr_lo, xerr_hi],
                        color=color, marker=marker, capsize=3, capthick=1,
                        label=label, zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg Latency (ms)')
    ax.set_title(f'Throughput vs Average Latency\n{WORKLOAD}', fontsize=11)
    ax.legend(loc='upper left', fontsize=8)
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))


def plot_latency_breakdown(ax, rows):
    """Figure B: Latency breakdown at 3 concurrency levels.
    Bars = P50 height, lines on top = P99.
    baseline: S_p50 only; hybrid: S_p50 + W_p50.
    """
    n_groups = len(SELECTED_THREADS)
    # Per group: baseline(S), HT(S), HT(W), HO(S), HO(W) = 5 bars
    n_bars = 5
    bar_width = 0.13
    gap = 0.04  # gap between bars within a group

    # Bar definitions: (protocol, type_key_prefix, label)
    bar_defs = [
        ('curp-baseline', 's', 'CURP (S)'),
        ('curpht',        's', 'CURP-HT (S)'),
        ('curpht',        'w', 'CURP-HT (W)'),
        ('curpho',        's', 'CURP-HO (S)'),
        ('curpho',        'w', 'CURP-HO (W)'),
    ]

    group_spacing = n_bars * (bar_width + gap) + 0.25
    group_centers = np.arange(n_groups) * group_spacing

    for gi, t in enumerate(SELECTED_THREADS):
        for bi, (proto, typ, lbl) in enumerate(bar_defs):
            p50_key = f'{typ}_p50'
            p99_key = f'{typ}_p99'

            p50 = get_val_from_rows(rows, proto, t, p50_key)
            p99 = get_val_from_rows(rows, proto, t, p99_key)

            if p50 is None:
                continue

            color = PROTOCOL_COLORS[proto]
            hatch = '//' if typ == 'w' else ''
            x = group_centers[gi] + (bi - (n_bars - 1) / 2) * (bar_width + gap)

            ax.bar(x, p50, bar_width,
                   color=color, hatch=hatch,
                   edgecolor='black', linewidth=0.5,
                   label=lbl if gi == 0 else '',
                   alpha=0.85, zorder=3)

            # P99 indicator: vertical line from bar top + horizontal cap
            if p99 is not None:
                ax.plot([x, x], [p50, p99],
                        color='black', linewidth=0.8, zorder=4)
                ax.plot([x - bar_width * 0.35, x + bar_width * 0.35],
                        [p99, p99],
                        color='black', linewidth=1.5, zorder=4)

    ax.set_xticks(group_centers)
    ax.set_xticklabels([f't = {t}' for t in SELECTED_THREADS])
    ax.set_ylabel('Latency (ms)')
    ax.set_title(f'Latency Breakdown (bar = P50, line = P99)\n{WORKLOAD}', fontsize=11)
    ax.legend(loc='upper left', fontsize=7, ncol=2,
              handlelength=1.5, columnspacing=1.0)
    ax.set_ylim(bottom=0)
    ax.grid(axis='y', alpha=0.3, linestyle='--')
    ax.grid(axis='x', visible=False)


def main():
    base = base_dir()
    csv_paths = [
        os.path.join(base, 'results', f'eval-5r-phase78-run{i}', 'summary-exp3.1.csv')
        for i in range(1, 4)
    ]
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_multi_run_csv(csv_paths)

    # Figure A: Throughput vs Weighted-Average Latency
    fig, ax = plt.subplots(1, 1, figsize=(7, 4.5))
    plot_tput_vs_avg(ax, rows)
    plt.tight_layout()
    save_figure(fig, out_dir, 'exp3.1-5r-throughput-avg-latency')

    # Figure B: Latency Breakdown at 3 concurrency levels
    fig, ax = plt.subplots(1, 1, figsize=(7, 4.5))
    plot_latency_breakdown(ax, rows)
    plt.tight_layout()
    save_figure(fig, out_dir, 'exp3.1-5r-latency-breakdown')


if __name__ == '__main__':
    main()
