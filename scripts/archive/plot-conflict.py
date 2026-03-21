"""
Plot conflict rate sensitivity: throughput and latency vs conflict level.

Generates a 2x2 figure:
  (a) Throughput vs conflict (5% writes)
  (b) Throughput vs conflict (50% writes)
  (c) Strong P50 latency vs conflict (5% writes)
  (d) Strong P50 latency vs conflict (50% writes)

Usage: python3 scripts/plot-conflict.py
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import (
    setup_style, load_csv_optional, get_val,
    PROTOCOL_COLORS, PROTOCOL_MARKERS, PROTOCOL_LABELS,
    kops_formatter, save_figure, base_dir,
)
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

RESULTS_DIR = os.path.join(base_dir(), 'results', 'eval-dist-20260307-w5')
OUT_DIR = os.path.join(base_dir(), 'evaluation', 'plots')

# Conflict level ordering for x-axis
CONFLICT_ORDER = ['uniform', 'mild', 'moderate', 'high', 'hotspot']
CONFLICT_LABELS = ['Uniform\n(s=0)', 'Mild\n(s=0.5)', 'Moderate\n(s=0.99)', 'High\n(s=1.5)', 'Hotspot\n(KS=1K)']

PROTOCOLS = ['epaxos', 'curp', 'raft']
PROTOCOL_KEY_MAP = {
    'curp': 'curp-baseline',  # Map to plot_style key
}


def load_conflict_data(csv_path):
    """Load conflict CSV and organize by protocol."""
    rows = load_csv_optional(csv_path)
    if not rows:
        return {}
    data = {}
    for row in rows:
        proto = row['protocol']
        level = row['conflict_level']
        if proto not in data:
            data[proto] = {}
        data[proto][level] = row
    return data


def extract_series(data, proto, metric):
    """Extract ordered series for a protocol and metric."""
    if proto not in data:
        return [], []
    xs, ys = [], []
    for i, level in enumerate(CONFLICT_ORDER):
        if level in data[proto]:
            val = get_val(data[proto][level], metric)
            if val is not None:
                xs.append(i)
                ys.append(val)
    return xs, ys


def style_key(proto):
    """Map protocol name to plot_style key."""
    return PROTOCOL_KEY_MAP.get(proto, proto)


def plot_panel(ax, data, metric, ylabel, title):
    """Plot one panel with all protocols."""
    for proto in PROTOCOLS:
        sk = style_key(proto)
        xs, ys = extract_series(data, proto, metric)
        if not xs:
            continue
        ax.plot(xs, ys,
                color=PROTOCOL_COLORS[sk],
                marker=PROTOCOL_MARKERS[sk],
                label=PROTOCOL_LABELS[sk],
                linewidth=2.0, markersize=8)

    ax.set_xticks(range(len(CONFLICT_ORDER)))
    ax.set_xticklabels(CONFLICT_LABELS, fontsize=8)
    ax.set_ylabel(ylabel)
    ax.set_title(title, fontsize=11, fontweight='bold')


def main():
    setup_style()

    # Load both datasets
    w5_path = os.path.join(RESULTS_DIR, 'summary-conflict.csv')
    w50_path = os.path.join(RESULTS_DIR, 'summary-conflict-w50.csv')

    data_w5 = load_conflict_data(w5_path)
    data_w50 = load_conflict_data(w50_path)

    if not data_w5 and not data_w50:
        print("ERROR: No conflict data found")
        return

    # Determine layout based on available data
    has_w50 = bool(data_w50)

    if has_w50:
        fig, axes = plt.subplots(2, 2, figsize=(10, 7))
        # (a) Throughput — 5% writes
        plot_panel(axes[0, 0], data_w5, 'throughput',
                   'Throughput (Kops/s)', '(a) Throughput (5% writes)')
        axes[0, 0].yaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

        # (b) Throughput — 50% writes
        plot_panel(axes[0, 1], data_w50, 'throughput',
                   'Throughput (Kops/s)', '(b) Throughput (50% writes)')
        axes[0, 1].yaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

        # (c) Latency — 5% writes
        plot_panel(axes[1, 0], data_w5, 's_p50',
                   'Strong P50 Latency (ms)', '(c) Strong P50 (5% writes)')

        # (d) Latency — 50% writes
        plot_panel(axes[1, 1], data_w50, 's_p50',
                   'Strong P50 Latency (ms)', '(d) Strong P50 (50% writes)')

        # Legend on top
        handles, labels = axes[0, 0].get_legend_handles_labels()
        fig.legend(handles, labels, loc='upper center', ncol=len(PROTOCOLS),
                   bbox_to_anchor=(0.5, 1.02), fontsize=10)

        fig.suptitle('Conflict Rate Sensitivity (t=32, strong-only)',
                     fontsize=13, fontweight='bold', y=1.06)
    else:
        fig, axes = plt.subplots(1, 2, figsize=(10, 4))
        # (a) Throughput — 5% writes
        plot_panel(axes[0], data_w5, 'throughput',
                   'Throughput (Kops/s)', '(a) Throughput (5% writes)')
        axes[0].yaxis.set_major_formatter(ticker.FuncFormatter(kops_formatter))

        # (b) Latency — 5% writes
        plot_panel(axes[1], data_w5, 's_p50',
                   'Strong P50 Latency (ms)', '(b) Strong P50 (5% writes)')

        handles, labels = axes[0].get_legend_handles_labels()
        fig.legend(handles, labels, loc='upper center', ncol=len(PROTOCOLS),
                   bbox_to_anchor=(0.5, 1.02), fontsize=10)

        fig.suptitle('Conflict Rate Sensitivity (t=32, strong-only)',
                     fontsize=13, fontweight='bold', y=1.08)

    plt.tight_layout()
    save_figure(fig, OUT_DIR, 'conflict-sensitivity')

    # Also generate a P99 version for appendix
    if has_w50:
        fig2, axes2 = plt.subplots(1, 2, figsize=(10, 4))

        plot_panel(axes2[0], data_w5, 's_p99',
                   'Strong P99 Latency (ms)', '(a) Strong P99 (5% writes)')
        plot_panel(axes2[1], data_w50, 's_p99',
                   'Strong P99 Latency (ms)', '(b) Strong P99 (50% writes)')

        handles, labels = axes2[0].get_legend_handles_labels()
        fig2.legend(handles, labels, loc='upper center', ncol=len(PROTOCOLS),
                    bbox_to_anchor=(0.5, 1.02), fontsize=10)
        fig2.suptitle('Conflict Rate Sensitivity — P99 (t=32, strong-only)',
                      fontsize=13, fontweight='bold', y=1.08)
        plt.tight_layout()
        save_figure(fig2, OUT_DIR, 'conflict-sensitivity-p99')

    print("\nDone!")


if __name__ == '__main__':
    main()
