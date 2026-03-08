#!/usr/bin/env python3
"""
Exp 3.2: T Property Verification — Strong Latency Stability

Generates two figures:
1. Strong P50 latency vs weak ratio (T property validation)
2. Total throughput vs weak ratio

Each figure has two subplots: distributed (RTT=50ms) and local (RTT=100ms).
"""

import csv
import os
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

plt.rcParams.update({
    'font.family': 'serif',
    'font.size': 11,
    'axes.labelsize': 13,
    'axes.titlesize': 14,
    'legend.fontsize': 10,
    'xtick.labelsize': 10,
    'ytick.labelsize': 10,
    'figure.dpi': 300,
    'savefig.dpi': 300,
    'savefig.bbox': 'tight',
    'lines.linewidth': 2.0,
    'lines.markersize': 8,
})

COLORS = {
    'curpho': '#2171B5',  # blue
    'curpht': '#238B45',  # green
    'raftht': '#D94701',  # orange
}
MARKERS = {
    'curpho': 'o',
    'curpht': 'D',
    'raftht': 's',
}
LABELS = {
    'curpho': 'CURP-HO',
    'curpht': 'CURP-HT',
    'raftht': 'Raft-HT',
}

def load_csv(path):
    rows = []
    with open(path, newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)
    return rows

def extract_series(rows, protocol):
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['weak_ratio']))
    weak_ratios = [int(r['weak_ratio']) for r in filtered]
    s_p50 = [float(r['s_p50']) for r in filtered]
    throughput = [float(r['throughput']) for r in filtered]
    return weak_ratios, s_p50, throughput

def plot_latency_subplot(ax, rows, title, rtt_label):
    for proto in ['raftht', 'curpht', 'curpho']:
        weak_ratios, s_p50, _ = extract_series(rows, proto)
        ax.plot(weak_ratios, s_p50,
                color=COLORS[proto], marker=MARKERS[proto],
                label=LABELS[proto], zorder=3)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Strong P50 Latency (ms)')
    ax.set_title(f'{title} ({rtt_label})')
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='best')
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)

def plot_throughput_subplot(ax, rows, title, rtt_label):
    for proto in ['raftht', 'curpht', 'curpho']:
        weak_ratios, _, throughput = extract_series(rows, proto)
        ax.plot(weak_ratios, [t/1000 for t in throughput],
                color=COLORS[proto], marker=MARKERS[proto],
                label=LABELS[proto], zorder=3)

    ax.set_xlabel('Weak Operation Ratio (%)')
    ax.set_ylabel('Throughput (Kops/sec)')
    ax.set_title(f'{title} ({rtt_label})')
    ax.set_xticks([0, 25, 50, 75, 100])
    ax.legend(loc='upper left')
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_xlim(-5, 105)
    ax.set_ylim(bottom=0)

def main():
    base = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    dist_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.2.csv')
    local_csv = os.path.join(base, 'results', 'eval-local-20260307-final3', 'summary-exp3.2.csv')
    out_dir = os.path.join(base, 'plots')
    os.makedirs(out_dir, exist_ok=True)

    dist_rows = load_csv(dist_csv)
    local_rows = load_csv(local_csv)

    # Figure 1: Strong P50 Latency (T property validation)
    fig1, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))
    plot_latency_subplot(ax1, dist_rows, 'T Property: Strong Latency', 'RTT = 50 ms')
    plot_latency_subplot(ax2, local_rows, 'T Property: Strong Latency', 'RTT = 100 ms')
    plt.tight_layout(w_pad=3)
    for ext in ['pdf', 'png']:
        out = os.path.join(out_dir, f'exp3.2-t-property-latency.{ext}')
        fig1.savefig(out)
        print(f'Saved: {out}')
    plt.close(fig1)

    # Figure 2: Throughput scaling
    fig2, (ax3, ax4) = plt.subplots(1, 2, figsize=(12, 4.5))
    plot_throughput_subplot(ax3, dist_rows, 'Weak Ratio Sweep: Throughput', 'RTT = 50 ms')
    plot_throughput_subplot(ax4, local_rows, 'Weak Ratio Sweep: Throughput', 'RTT = 100 ms')
    plt.tight_layout(w_pad=3)
    for ext in ['pdf', 'png']:
        out = os.path.join(out_dir, f'exp3.2-t-property-throughput.{ext}')
        fig2.savefig(out)
        print(f'Saved: {out}')
    plt.close(fig2)

if __name__ == '__main__':
    main()
