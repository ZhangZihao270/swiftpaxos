#!/usr/bin/env python3
"""
CDF plots for 5-replica latency distributions.

Reads latencies.json files from 5-replica experiment directories and generates
CDF plots comparing latency distributions across protocols.

Uses Phase 78 data (run1 for latency distributions).

Figures:
  - Strong latency CDF at moderate load (t=32)
  - Weak latency CDF at moderate load (t=32)
  - Combined 2-panel CDF figure
  - Weak operation breakdown (read vs write)
  - T property CDF (strong latency at different weak ratios)
"""

import json
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import *
import numpy as np

CDF_THREADS = 32
WORKLOAD = '95/5 R/W, 50% weak, Zipfian, 5 replicas, RTT=50ms'


def load_latencies(exp_dir, protocol, threads):
    """Load latencies.json from experiment output directory."""
    path = os.path.join(exp_dir, protocol, f't{threads}', 'latencies.json')
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)


def cdf_xy(values):
    if not values:
        return [], []
    n = len(values)
    y = np.arange(1, n + 1) / n
    return values, y


def plot_cdf_panel(ax, data_dict, category, title):
    all_p999 = []
    for proto in ['curpho', 'curpht', 'curp-baseline']:
        if proto not in data_dict or data_dict[proto] is None:
            continue
        lat = data_dict[proto]

        if category == 'strong':
            vals = sorted(lat.get('strong_write', []) + lat.get('strong_read', []))
        else:
            vals = sorted(lat.get('weak_write', []) + lat.get('weak_read', []))

        if not vals:
            continue

        x, y = cdf_xy(vals)
        ax.plot(x, y, color=PROTOCOL_COLORS[proto],
                label=PROTOCOL_LABELS[proto], linewidth=2, zorder=3)

        p999_idx = min(int(len(vals) * 0.999), len(vals) - 1)
        all_p999.append(vals[p999_idx])

        p50 = vals[len(vals) // 2]
        ax.axvline(p50, color=PROTOCOL_COLORS[proto], linestyle=':', alpha=0.4, linewidth=1)

    ax.set_xlabel('Latency (ms)')
    ax.set_ylabel('CDF')
    ax.set_title(title, fontsize=11)
    ax.set_ylim(0, 1.02)
    ax.set_xlim(left=0)
    if all_p999:
        ax.set_xlim(right=max(all_p999) * 1.1)
    ax.legend(loc='lower right', fontsize=8)
    ax.axhline(0.5, color='gray', linestyle='--', alpha=0.3, linewidth=0.8)
    ax.axhline(0.99, color='gray', linestyle='--', alpha=0.3, linewidth=0.8)
    ax.text(ax.get_xlim()[1] * 0.98, 0.5, 'P50', ha='right', va='bottom',
            fontsize=7, color='gray', alpha=0.6)
    ax.text(ax.get_xlim()[1] * 0.98, 0.99, 'P99', ha='right', va='bottom',
            fontsize=7, color='gray', alpha=0.6)


def main():
    base = base_dir()
    # Phase 78 data: use run1 for CDF (latency distributions)
    exp31_dir = os.path.join(base, 'results', 'eval-5r-phase78-run1', 'exp3.1')
    exp32_dir = os.path.join(base, 'results', 'eval-5r-exp3.2-phase78-20260309', 'exp3.2')
    out_dir = os.path.join(base, 'evaluation', 'plots')

    print(f'Exp 3.1 dir: {exp31_dir}')
    print(f'Exp 3.2 dir: {exp32_dir}')

    setup_style()

    # Load latency data
    data = {}
    for proto in ['curpho', 'curpht', 'curp-baseline']:
        lat = load_latencies(exp31_dir, proto, CDF_THREADS)
        if lat is not None:
            data[proto] = lat
            total = sum(len(lat.get(k, [])) for k in ['strong_write', 'strong_read', 'weak_write', 'weak_read'])
            print(f'  {proto}: {total} samples loaded')
        else:
            print(f'  {proto}: no latency data found')

    if not data:
        print('No latency data found.')
        return

    # Two-panel CDF figure
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 4.5))
    plot_cdf_panel(ax1, data, 'strong',
                   f'(a) Strong Latency CDF (t={CDF_THREADS})\n{WORKLOAD}')
    weak_data = {k: v for k, v in data.items() if k in ('curpho', 'curpht')}
    plot_cdf_panel(ax2, weak_data, 'weak',
                   f'(b) Weak Latency CDF (t={CDF_THREADS})\n{WORKLOAD}')
    plt.tight_layout(w_pad=3)
    save_figure(fig, out_dir, 'cdf-5r-latency')

    # Individual strong-only CDF
    fig2, ax = plt.subplots(figsize=(7, 4.5))
    plot_cdf_panel(ax, data, 'strong',
                   f'Strong Latency CDF (t={CDF_THREADS}, {WORKLOAD})')
    plt.tight_layout()
    save_figure(fig2, out_dir, 'cdf-5r-strong-latency')

    # Weak breakdown
    plot_weak_breakdown(data, out_dir)

    # T property CDF
    plot_t_property_cdf(exp32_dir, out_dir)


def plot_weak_breakdown(data, out_dir):
    hybrid_protos = ['curpho', 'curpht']
    available = [p for p in hybrid_protos if p in data]
    if not available:
        return

    fig, axes = plt.subplots(1, len(available), figsize=(5 * len(available), 4.5))
    if len(available) == 1:
        axes = [axes]

    read_color = WONG['blue']
    write_color = WONG['red']

    for ax, proto in zip(axes, available):
        lat = data[proto]
        all_p999 = []

        for key, label, color in [
            ('weak_read', 'Weak Read', read_color),
            ('weak_write', 'Weak Write', write_color),
        ]:
            vals = lat.get(key, [])
            if not vals:
                continue
            x, y = cdf_xy(vals)
            ax.plot(x, y, color=color, label=label, linewidth=2)
            p999_idx = min(int(len(vals) * 0.999), len(vals) - 1)
            all_p999.append(vals[p999_idx])

        ax.set_xlabel('Latency (ms)')
        ax.set_ylabel('CDF')
        ax.set_title(f'{PROTOCOL_LABELS[proto]}', fontsize=12)
        ax.set_ylim(0, 1.02)
        ax.set_xlim(left=0)
        if all_p999:
            ax.set_xlim(right=max(all_p999) * 1.1)
        ax.legend(loc='lower right', fontsize=9)
        ax.axhline(0.5, color='gray', linestyle='--', alpha=0.3, linewidth=0.8)
        ax.axhline(0.99, color='gray', linestyle='--', alpha=0.3, linewidth=0.8)

    fig.suptitle(f'Weak Operation Breakdown (t={CDF_THREADS}, {WORKLOAD})',
                 fontsize=12, y=1.02)
    plt.tight_layout()
    save_figure(fig, out_dir, 'cdf-5r-weak-breakdown')


def plot_t_property_cdf(exp32_dir, out_dir):
    if not os.path.isdir(exp32_dir):
        print(f'Exp 3.2 dir not found: {exp32_dir}')
        return

    protos = ['curpho', 'curpht', 'curp-baseline']
    ratios = [0, 50, 100]
    ratio_styles = {
        0:   ('-',  1.0, 'w0 (all strong)'),
        50:  ('--', 0.8, 'w50 (50% weak)'),
        100: (':',  0.6, 'w100 (all weak)'),
    }

    fig, axes = plt.subplots(1, 3, figsize=(15, 4.5))

    for ax, proto in zip(axes, protos):
        all_p999 = []
        for ratio in ratios:
            path = os.path.join(exp32_dir, proto, f'w{ratio}', 'latencies.json')
            if not os.path.exists(path):
                print(f'  Missing: {path}')
                continue
            with open(path) as f:
                lat = json.load(f)
            vals = sorted(lat.get('strong_write', []) + lat.get('strong_read', []))
            if not vals:
                continue

            ls, alpha, label = ratio_styles[ratio]
            x, y = cdf_xy(vals)
            ax.plot(x, y, color=PROTOCOL_COLORS[proto],
                    linestyle=ls, alpha=alpha, label=label, linewidth=2)

            p999_idx = min(int(len(vals) * 0.999), len(vals) - 1)
            all_p999.append(vals[p999_idx])

        ax.set_xlabel('Latency (ms)')
        ax.set_ylabel('CDF')
        ax.set_title(PROTOCOL_LABELS[proto], fontsize=12)
        ax.set_ylim(0, 1.02)
        ax.set_xlim(left=0)
        if all_p999:
            ax.set_xlim(right=max(all_p999) * 1.1)
        ax.legend(loc='lower right', fontsize=8)
        ax.axhline(0.5, color='gray', linestyle='--', alpha=0.3, linewidth=0.8)
        ax.axhline(0.99, color='gray', linestyle='--', alpha=0.3, linewidth=0.8)

    fig.suptitle('T Property: Strong Latency CDF vs Weak Ratio\n'
                 '95/5 R/W, t=32, Zipfian, 5 replicas, RTT=50ms',
                 fontsize=12, y=1.04)
    plt.tight_layout()
    save_figure(fig, out_dir, 'cdf-5r-t-property')


if __name__ == '__main__':
    main()
