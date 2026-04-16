#!/usr/bin/env python3
"""
Combined figure: TAO (left) + YCSB-B/Cross (right) throughput vs latency.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

ALL_PROTOS = ['raft-baseline', 'raftht',
              'epaxos-baseline', 'epaxosho',
              'curp-baseline', 'curpht', 'curpho',
              'mongotunable',
              'pileus', 'pileusht']


def plot_tput_lat(ax, rows, protocols):
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
            ax.plot(x, y, color=color, marker=marker, markersize=13,
                    linewidth=3.5, label=label, zorder=z)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Avg Latency (ms)')
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))


def main():
    base = base_dir()
    cross_csv = os.path.join(base, 'results', 'latest', 'exp-cross.csv')
    tao_csv   = os.path.join(base, 'results', 'latest', 'exp-tao.csv')
    out_dir   = os.path.join(base, 'evaluation', 'plots')

    setup_style()
    cross_rows = load_csv(cross_csv)
    tao_rows   = load_csv(tao_csv)

    fig, axes = plt.subplots(1, 2, figsize=(16, 5.5))

    plot_tput_lat(axes[0], tao_rows, ALL_PROTOS)
    plot_tput_lat(axes[1], cross_rows, ALL_PROTOS)

    # Legend inside panel (a) only
    axes[0].legend(loc='upper right', ncol=2, fontsize=19, framealpha=0.9)

    labels_cap = ['(a) TAO', '(b) YCSB-B']
    for i, ax in enumerate(axes):
        ax.text(0.5, -0.32, labels_cap[i], transform=ax.transAxes,
                fontsize=22, fontweight='bold', ha='center')

    plt.tight_layout(w_pad=2)
    plt.subplots_adjust(bottom=0.22)
    save_figure(fig, out_dir, 'exp-combined-throughput-latency')


if __name__ == '__main__':
    main()
