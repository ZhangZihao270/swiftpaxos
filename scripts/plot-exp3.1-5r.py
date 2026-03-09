#!/usr/bin/env python3
"""
Exp 3.1 (5-Replica): CURP-HO vs CURP-HT vs Baseline — Throughput vs Latency
Single-panel figure for 5-replica distributed results (RTT=50ms).
Uses Phase 78 data: median of 3 runs with min/max error bands.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, 50% weak, Zipfian, 5 replicas'

def plot_figure(ax, rows, percentile='p50'):
    label_suffix = 'P50' if percentile == 'p50' else 'P99'
    s_key = f's_{percentile}'
    w_key = f'w_{percentile}'

    for proto in ['curpho', 'curpht', 'curp-baseline']:
        data = extract_tput_latency_with_errbars(rows, proto)
        color = PROTOCOL_COLORS[proto]
        marker = PROTOCOL_MARKERS[proto]
        label = PROTOCOL_LABELS[proto]

        # Build aligned arrays for strong ops
        xs, ys, x_los, x_his = [], [], [], []
        for i in range(len(data['throughput'])):
            xv = data['throughput'][i]
            yv = data[s_key][i]
            if xv is not None and yv is not None and xv > 0:
                xs.append(xv)
                ys.append(yv)
                xlo = (data['throughput_lo'] or [])[i] if data.get('throughput_lo') else None
                xhi = (data['throughput_hi'] or [])[i] if data.get('throughput_hi') else None
                x_los.append(xlo if xlo is not None else xv)
                x_his.append(xhi if xhi is not None else xv)

        # Pareto trim (up to peak throughput)
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
                        label=f'{label} (strong)', zorder=3)

        # Weak ops (dashed) — only for hybrid protocols
        if proto != 'curp-baseline':
            xs, ys = [], []
            for i in range(len(data['throughput'])):
                xv = data['throughput'][i]
                yv = data[w_key][i]
                if xv is not None and yv is not None and xv > 0:
                    xs.append(xv)
                    ys.append(yv)
            if xs:
                peak_idx = max(range(len(xs)), key=lambda j: xs[j])
                xs = xs[:peak_idx + 1]
                ys = ys[:peak_idx + 1]
                ax.plot(xs, ys, color=color, marker=marker, markersize=5,
                        linestyle='--', alpha=0.7,
                        label=f'{label} (weak)', zorder=2)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel(f'{label_suffix} Latency (ms)')
    ax.set_title(f'CURP Throughput vs Latency (5 replicas, RTT = 50 ms)\n{WORKLOAD}', fontsize=11)
    ax.legend(loc='upper left', fontsize=8, ncol=1)
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

def main():
    base = base_dir()
    # Phase 78: 3 reproducible runs
    csv_paths = [
        os.path.join(base, 'results', f'eval-5r-phase78-run{i}', 'summary-exp3.1.csv')
        for i in range(1, 4)
    ]
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    rows = load_multi_run_csv(csv_paths)

    for pct, name_suffix in [('p50', ''), ('p99', '-p99')]:
        fig, ax = plt.subplots(1, 1, figsize=(7, 4.5))
        plot_figure(ax, rows, pct)
        plt.tight_layout()
        save_figure(fig, out_dir, f'exp3.1-5r-throughput-latency{name_suffix}')

if __name__ == '__main__':
    main()
