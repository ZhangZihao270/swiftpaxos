#!/usr/bin/env python3
"""
Hero Figure: All Protocols — Throughput vs Latency (Distributed)
Single figure combining Exp 1.1 and 3.1 data to show the complete
HOT trade-off landscape. Shows strong P50 latency for all 5 protocols.
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

WORKLOAD = '95/5 R/W, 50% weak, Zipfian, RTT = 50 ms'

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
    # Protocols from Exp 3.1 (CURP family)
    for proto in ['curpho', 'curpht', 'curp-baseline']:
        data = extract_tput_latency(exp31_rows, proto)
        x, y = clean_pairs(data['throughput'], data['s_p50'])
        ax_strong.plot(x, y,
                       color=PROTOCOL_COLORS[proto],
                       marker=PROTOCOL_MARKERS[proto],
                       label=f'{PROTOCOL_LABELS[proto]}',
                       zorder=3)

    # Protocols from Exp 1.1 (Raft family)
    for proto in ['raftht', 'raft']:
        data = extract_tput_latency(exp11_rows, proto)
        x, y = clean_pairs(data['throughput'], data['s_p50'])
        ax_strong.plot(x, y,
                       color=PROTOCOL_COLORS[proto],
                       marker=PROTOCOL_MARKERS[proto],
                       label=f'{PROTOCOL_LABELS[proto]}',
                       zorder=3)

    ax_strong.set_xlabel('Throughput (Kops/sec)')
    ax_strong.set_ylabel('Strong P50 Latency (ms)')
    ax_strong.set_title(f'Strong Operations\n{WORKLOAD}', fontsize=11)
    ax_strong.legend(loc='upper left', fontsize=9)
    ax_strong.set_xlim(left=0)
    ax_strong.set_ylim(bottom=0)
    ax_strong.xaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

    # ── Right panel: Weak P50 latency ──────────────────────────────────
    # Only hybrid protocols have weak ops
    for proto in ['curpho', 'curpht']:
        data = extract_tput_latency(exp31_rows, proto)
        x, y = clean_pairs(data['throughput'], data['w_p50'])
        ax_weak.plot(x, y,
                     color=PROTOCOL_COLORS[proto],
                     marker=PROTOCOL_MARKERS[proto],
                     label=f'{PROTOCOL_LABELS[proto]}',
                     zorder=3)

    data = extract_tput_latency(exp11_rows, 'raftht')
    x, y = clean_pairs(data['throughput'], data['w_p50'])
    ax_weak.plot(x, y,
                 color=PROTOCOL_COLORS['raftht'],
                 marker=PROTOCOL_MARKERS['raftht'],
                 label='Raft-HT',
                 zorder=3)

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
