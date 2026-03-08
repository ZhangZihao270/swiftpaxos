#!/usr/bin/env python3
"""
Hero Figure: All Protocols — Throughput vs Latency (Distributed)
Single figure combining Exp 1.1 and 3.1 data to show the complete
HOT trade-off landscape. Shows strong P50 latency for all 5 protocols.
Curves trimmed at peak throughput. Peak annotated.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, 50% weak, Zipfian, RTT = 50 ms'

def annotate_peak(ax, x, y, label, color, offset=(10, 5)):
    """Add a small annotation at the peak throughput point."""
    if not x:
        return
    peak_idx = len(x) - 1  # after pareto_frontier, last point is peak
    peak_x, peak_y = x[peak_idx], y[peak_idx]
    ax.annotate(f'{peak_x/1000:.0f}K',
                xy=(peak_x, peak_y),
                xytext=offset,
                textcoords='offset points',
                fontsize=8, color=color, fontweight='bold',
                ha='left', va='bottom')

def main():
    base = base_dir()
    exp11_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp1.1.csv')
    exp31_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.1.csv')
    out_dir = os.path.join(base, 'plots')

    setup_style()
    exp11_rows = load_csv(exp11_csv)
    exp31_rows = load_csv(exp31_csv)

    fig, (ax_strong, ax_weak) = plt.subplots(1, 2, figsize=(13, 5))

    # ── Left panel: Strong P50 latency ─────────────────────────────────
    # Custom annotation offsets to avoid overlap
    ann_offsets = {
        'curpho': (5, 8),
        'curpht': (-40, 8),
        'curp-baseline': (5, -15),
        'raftht': (5, 8),
        'raft': (5, -15),
    }

    for proto, src_rows in [('curpho', exp31_rows), ('curpht', exp31_rows),
                             ('curp-baseline', exp31_rows),
                             ('raftht', exp11_rows), ('raft', exp11_rows)]:
        data = extract_tput_latency(src_rows, proto)
        x, y = clean_pairs(data['throughput'], data['s_p50'])
        x, y = pareto_frontier(x, y)
        ax_strong.plot(x, y,
                       color=PROTOCOL_COLORS[proto],
                       marker=PROTOCOL_MARKERS[proto],
                       label=PROTOCOL_LABELS[proto],
                       zorder=3)
        annotate_peak(ax_strong, x, y, proto, PROTOCOL_COLORS[proto],
                      offset=ann_offsets.get(proto, (10, 5)))

    ax_strong.set_xlabel('Throughput (Kops/sec)')
    ax_strong.set_ylabel('Strong P50 Latency (ms)')
    ax_strong.set_title(f'Strong Operations\n{WORKLOAD}', fontsize=11)
    ax_strong.legend(loc='upper left', fontsize=9)
    ax_strong.set_xlim(left=0)
    ax_strong.set_ylim(bottom=0)
    ax_strong.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

    # ── Right panel: Weak P50 latency ──────────────────────────────────
    for proto, src_rows in [('curpho', exp31_rows), ('curpht', exp31_rows),
                             ('raftht', exp11_rows)]:
        data = extract_tput_latency(src_rows, proto)
        x, y = clean_pairs(data['throughput'], data['w_p50'])
        x, y = pareto_frontier(x, y)
        ax_weak.plot(x, y,
                     color=PROTOCOL_COLORS[proto],
                     marker=PROTOCOL_MARKERS[proto],
                     label=PROTOCOL_LABELS[proto],
                     zorder=3)
        annotate_peak(ax_weak, x, y, proto, PROTOCOL_COLORS[proto])

    ax_weak.set_xlabel('Throughput (Kops/sec)')
    ax_weak.set_ylabel('Weak P50 Latency (ms)')
    ax_weak.set_title(f'Weak Operations\n{WORKLOAD}', fontsize=11)
    ax_weak.legend(loc='upper left', fontsize=9)
    ax_weak.set_xlim(left=0)
    ax_weak.set_ylim(bottom=0)
    ax_weak.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

    plt.tight_layout(w_pad=3)
    save_figure(fig, out_dir, 'hero-all-protocols')

if __name__ == '__main__':
    main()
