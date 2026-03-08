#!/usr/bin/env python3
"""
Exp 3.1: CURP-HO vs CURP-HT vs Baseline — Throughput vs Latency
Generates two subplots: distributed (RTT=50ms) and local (RTT=100ms).
Each subplot shows throughput (X) vs median latency (Y) with separate
curves for strong and weak operations.
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
    'legend.fontsize': 9,
    'xtick.labelsize': 10,
    'ytick.labelsize': 10,
    'figure.dpi': 300,
    'savefig.dpi': 300,
    'savefig.bbox': 'tight',
    'lines.linewidth': 2.0,
    'lines.markersize': 7,
})

COLORS = {
    'curpho_strong':   '#2171B5',  # dark blue
    'curpho_weak':     '#6BAED6',  # light blue
    'curpht_strong':   '#238B45',  # dark green
    'curpht_weak':     '#74C476',  # light green
    'baseline_strong': '#D94701',  # dark orange
}
MARKERS = {
    'curpho_strong':   'o',
    'curpho_weak':     's',
    'curpht_strong':   'D',
    'curpht_weak':     'v',
    'baseline_strong': '^',
}
LABELS = {
    'curpho_strong':   'CURP-HO (strong)',
    'curpho_weak':     'CURP-HO (weak)',
    'curpht_strong':   'CURP-HT (strong)',
    'curpht_weak':     'CURP-HT (weak)',
    'baseline_strong': 'CURP Baseline (strong-only)',
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
    filtered.sort(key=lambda r: int(r['threads']))
    tput = [float(r['throughput']) for r in filtered]
    s_p50 = [float(r['s_p50']) if r['s_p50'] != 'N/A' else None for r in filtered]
    w_p50 = [float(r['w_p50']) if r['w_p50'] != 'N/A' else None for r in filtered]
    return tput, s_p50, w_p50

def clean(xs, ys):
    xc, yc = [], []
    for x, y in zip(xs, ys):
        if x > 0 and y is not None:
            xc.append(x)
            yc.append(y)
    return xc, yc

def plot_subplot(ax, rows, title, rtt_label):
    for proto, proto_name in [('curpho', 'curpho'), ('curpht', 'curpht'), ('curp-baseline', 'baseline')]:
        tput, s_p50, w_p50 = extract_series(rows, proto)

        key_s = f'{proto_name}_strong'
        x, y = clean(tput, s_p50)
        ax.plot(x, y, color=COLORS[key_s], marker=MARKERS[key_s],
                label=LABELS[key_s], zorder=3)

        if proto_name != 'baseline':
            key_w = f'{proto_name}_weak'
            x, y = clean(tput, w_p50)
            ax.plot(x, y, color=COLORS[key_w], marker=MARKERS[key_w],
                    label=LABELS[key_w], linestyle='--', zorder=3)

    ax.set_xlabel('Throughput (Kops/sec)')
    ax.set_ylabel('Median Latency (ms)')
    ax.set_title(f'{title} ({rtt_label})')
    ax.legend(loc='upper left', ncol=1)
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_xlim(left=0)
    ax.set_ylim(bottom=0)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x/1000:.0f}'))

def main():
    base = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    dist_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.1.csv')
    local_csv = os.path.join(base, 'results', 'eval-local-20260307-final3', 'summary-exp3.1.csv')
    out_dir = os.path.join(base, 'plots')
    os.makedirs(out_dir, exist_ok=True)

    dist_rows = load_csv(dist_csv)
    local_rows = load_csv(local_csv)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))

    plot_subplot(ax1, dist_rows, 'Exp 3.1: CURP Throughput', 'RTT = 50 ms')
    plot_subplot(ax2, local_rows, 'Exp 3.1: CURP Throughput', 'RTT = 100 ms')

    plt.tight_layout(w_pad=3)

    out_pdf = os.path.join(out_dir, 'exp3.1-throughput-latency.pdf')
    out_png = os.path.join(out_dir, 'exp3.1-throughput-latency.png')
    fig.savefig(out_pdf)
    fig.savefig(out_png)
    print(f'Saved: {out_pdf}')
    print(f'Saved: {out_png}')
    plt.close()

if __name__ == '__main__':
    main()
