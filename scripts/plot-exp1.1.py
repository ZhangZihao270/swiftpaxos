#!/usr/bin/env python3
"""
Exp 1.1: Raft-HT vs Vanilla Raft — Throughput vs Latency
Generates two subplots: distributed (RTT=50ms) and local (RTT=100ms).
Each subplot shows throughput (X) vs median latency (Y) with separate
curves for strong and weak operations.
"""

import csv
import os
import sys
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

# ── Style ──────────────────────────────────────────────────────────────
plt.rcParams.update({
    'font.family': 'serif',
    'font.size': 11,
    'axes.labelsize': 13,
    'axes.titlesize': 14,
    'legend.fontsize': 9.5,
    'xtick.labelsize': 10,
    'ytick.labelsize': 10,
    'figure.dpi': 300,
    'savefig.dpi': 300,
    'savefig.bbox': 'tight',
    'lines.linewidth': 2.0,
    'lines.markersize': 7,
})

COLORS = {
    'raftht_strong': '#2171B5',   # dark blue
    'raftht_weak':   '#6BAED6',   # light blue
    'raft_strong':   '#D94701',   # dark orange
}
MARKERS = {
    'raftht_strong': 'o',
    'raftht_weak':   's',
    'raft_strong':   '^',
}
LABELS = {
    'raftht_strong': 'Raft-HT (strong)',
    'raftht_weak':   'Raft-HT (weak)',
    'raft_strong':   'Raft (strong-only)',
}

def load_csv(path):
    rows = []
    with open(path, newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)
    return rows

def extract_series(rows, protocol):
    """Return (throughputs, s_p50s, w_p50s) sorted by threads."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['threads']))
    tput = [float(r['throughput']) for r in filtered]
    s_p50 = [float(r['s_p50']) if r['s_p50'] != 'N/A' else None for r in filtered]
    w_p50 = [float(r['w_p50']) if r['w_p50'] != 'N/A' else None for r in filtered]
    return tput, s_p50, w_p50

def plot_subplot(ax, rows, title, rtt_label):
    tput_ht, s_p50_ht, w_p50_ht = extract_series(rows, 'raftht')
    tput_r, s_p50_r, _ = extract_series(rows, 'raft')

    # Filter out zero-throughput or None entries
    def clean(xs, ys):
        xc, yc = [], []
        for x, y in zip(xs, ys):
            if x > 0 and y is not None:
                xc.append(x)
                yc.append(y)
        return xc, yc

    x, y = clean(tput_ht, s_p50_ht)
    ax.plot(x, y, color=COLORS['raftht_strong'], marker=MARKERS['raftht_strong'],
            label=LABELS['raftht_strong'], zorder=3)

    x, y = clean(tput_ht, w_p50_ht)
    ax.plot(x, y, color=COLORS['raftht_weak'], marker=MARKERS['raftht_weak'],
            label=LABELS['raftht_weak'], zorder=3)

    x, y = clean(tput_r, s_p50_r)
    ax.plot(x, y, color=COLORS['raft_strong'], marker=MARKERS['raft_strong'],
            label=LABELS['raft_strong'], zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Median Latency (ms)')
    ax.set_title(f'{title} ({rtt_label})')
    ax.legend(loc='upper left')
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x/1000:.0f}'))

def main():
    base = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    dist_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp1.1.csv')
    local_csv = os.path.join(base, 'results', 'eval-local-20260307-final3', 'summary-exp1.1.csv')
    out_dir = os.path.join(base, 'plots')
    os.makedirs(out_dir, exist_ok=True)

    dist_rows = load_csv(dist_csv)
    local_rows = load_csv(local_csv)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))

    plot_subplot(ax1, dist_rows, 'Exp 1.1: Raft-HT vs Raft', 'RTT = 50 ms')
    plot_subplot(ax2, local_rows, 'Exp 1.1: Raft-HT vs Raft', 'RTT = 100 ms')

    plt.tight_layout(w_pad=3)

    out_pdf = os.path.join(out_dir, 'exp1.1-throughput-latency.pdf')
    out_png = os.path.join(out_dir, 'exp1.1-throughput-latency.png')
    fig.savefig(out_pdf)
    fig.savefig(out_png)
    print(f'Saved: {out_pdf}')
    print(f'Saved: {out_png}')
    plt.close()

if __name__ == '__main__':
    main()
