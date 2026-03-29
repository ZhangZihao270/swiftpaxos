#!/usr/bin/env python3
"""
Exp 2.3: EPaxos-HO Failure Recovery — Throughput over Time.

Layout (2×1):
  ┌───────────────────────────────┐
  │ Kill replica0 (co-located)   │
  ├───────────────────────────────┤
  │ Kill replica3 (non-client)   │
  └───────────────────────────────┘
"""

import csv
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *

RAFTHT_DIR = 'exp2.3-v3-20260311_191011'
EPAXOSHO_DIR = 'exp2.3-epaxosho-killr0-phase113g-v1'

RAFTHT_KILL_TIME = 45
EPAXOSHO_KILL_TIME = 45


def load_tput(base, result_dir):
    path = os.path.join(base, 'results', result_dir, 'tput-aggregated.csv')
    with open(path) as f:
        rows = list(csv.DictReader(f))
    tputs = [(int(r['timestamp']), int(r['total_ops'])) for r in rows]
    t0 = tputs[0][0]
    return [ts - t0 - 1 for ts, _ in tputs], [tp / 1000 for _, tp in tputs]


def plot_failure(ax, times, tputs, kill_time, title, color, kill_label='Kill leader'):
    ax.plot(times, tputs, color=color, linewidth=2.5, zorder=3)
    ax.axvline(x=kill_time, color='red', linestyle='--', linewidth=2, alpha=0.8, label=f'{kill_label} at t={kill_time}s')
    ax.set_ylabel('Throughput\n(Kops/sec)')
    ax.legend(loc='center right', fontsize=12)
    ax.set_xlim(20, 70)
    ax.set_ylim(bottom=0)
    ax.grid(True, alpha=0.3, linestyle='--')


def main():
    base = base_dir()
    out_dir = os.path.join(base, 'evaluation', 'plots')

    setup_style()

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(10, 4.5), sharex=False)

    # Top: Raft-HT (20-70s)
    t1, tp1 = load_tput(base, RAFTHT_DIR)
    t1, tp1 = zip(*[(t, tp) for t, tp in zip(t1, tp1) if 20 <= t <= 70])
    plot_failure(ax1, list(t1), list(tp1), RAFTHT_KILL_TIME,
                 'Raft-HT: Kill Leader', PROTOCOL_COLORS['raftht'], kill_label='Kill leader')

    # Bottom: EPaxos-HO (20-70s)
    t2, tp2 = load_tput(base, EPAXOSHO_DIR)
    t2, tp2 = zip(*[(t, tp) for t, tp in zip(t2, tp2) if 20 <= t <= 70])
    plot_failure(ax2, list(t2), list(tp2), EPAXOSHO_KILL_TIME,
                 'EPaxos-HO: Kill Replica', PROTOCOL_COLORS['epaxosho'], kill_label='Kill a replica')

    ax2.set_xlabel('Time (s)', labelpad=-2)

    ax1.text(0.5, -0.22, '(a) Raft-HT', transform=ax1.transAxes,
             fontsize=14, fontweight='bold', ha='center')
    ax2.text(0.5, -0.42, '(b) EPaxos-HO', transform=ax2.transAxes,
             fontsize=14, fontweight='bold', ha='center')

    plt.tight_layout(h_pad=0.5)
    plt.subplots_adjust(bottom=0.15)
    save_figure(fig, out_dir, 'exp2.3-failure-recovery')


if __name__ == '__main__':
    main()
