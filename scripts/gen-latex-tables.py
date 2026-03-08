#!/usr/bin/env python3
"""
Generate LaTeX tables from experiment CSV data for paper inclusion.
Outputs to plots/tables.tex (and prints to stdout).
"""

import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import load_csv, get_val, base_dir

def fmt_k(v):
    """Format throughput as X.XK or X,XXX."""
    if v >= 1000:
        return f'{v/1000:.1f}K'
    return f'{v:.0f}'

def fmt_ms(v):
    """Format latency in ms."""
    if v is None:
        return '---'
    if v < 1:
        return f'{v:.2f}'
    return f'{v:.0f}'

def get_peak(rows, protocol):
    """Get peak throughput row for a protocol."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    if not filtered:
        return None
    return max(filtered, key=lambda r: float(r['throughput']))

def table_peak_throughput(exp11_rows, exp31_rows):
    """Table 1: Peak throughput comparison (distributed)."""
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{Peak throughput comparison (RTT = 50\,ms, 95/5 R/W, 50\% weak, Zipfian).}')
    lines.append(r'\label{tab:peak-throughput}')
    lines.append(r'\begin{tabular}{lrrrrr}')
    lines.append(r'\toprule')
    lines.append(r'Protocol & Peak (Kops/s) & Threads & Strong P50 (ms) & Weak P50 (ms) & vs.\ Baseline \\')
    lines.append(r'\midrule')

    protocols = [
        ('CURP-HO',         'curpho',        exp31_rows),
        ('CURP-HT',         'curpht',        exp31_rows),
        ('Raft-HT',         'raftht',        exp11_rows),
        ('CURP (baseline)',  'curp-baseline', exp31_rows),
        ('Raft',            'raft',           exp11_rows),
    ]

    baseline_peak = float(get_peak(exp31_rows, 'curp-baseline')['throughput'])

    for label, proto, rows in protocols:
        row = get_peak(rows, proto)
        tput = float(row['throughput'])
        threads = int(row['threads'])
        s_p50 = get_val(row, 's_p50')
        w_p50 = get_val(row, 'w_p50')
        speedup = tput / baseline_peak

        # Use appropriate baseline for Raft family
        if proto == 'raft':
            speedup_str = '---'
        elif proto == 'curp-baseline':
            speedup_str = '1.0$\\times$'
        else:
            speedup_str = f'{speedup:.1f}$\\times$'

        lines.append(f'{label} & {fmt_k(tput)} & {threads}$\\times$3 & {fmt_ms(s_p50)} & {fmt_ms(w_p50)} & {speedup_str} \\\\')

    lines.append(r'\bottomrule')
    lines.append(r'\end{tabular}')
    lines.append(r'\end{table}')
    return '\n'.join(lines)

def table_t_property(exp32_rows):
    """Table 2: T-property validation (distributed)."""
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{T-property validation: strong P50 latency across weak ratios (RTT = 50\,ms, 50/50 R/W, $t$=8).}')
    lines.append(r'\label{tab:t-property}')
    lines.append(r'\begin{tabular}{lrrrrrl}')
    lines.append(r'\toprule')
    lines.append(r'Protocol & $w$=0\% & $w$=25\% & $w$=50\% & $w$=75\% & $w$=100\% & T satisfied? \\')
    lines.append(r'\midrule')

    for proto, label in [('raftht', 'Raft-HT'), ('curpht', 'CURP-HT'), ('curpho', 'CURP-HO')]:
        filtered = [r for r in exp32_rows if r['protocol'] == proto]
        filtered.sort(key=lambda r: int(r['weak_ratio']))
        vals = [float(r['s_p50']) for r in filtered]
        # T property: check if max deviation from mean is < 15%
        mean_val = sum(vals) / len(vals)
        max_dev = max(abs(v - mean_val) / mean_val for v in vals)
        t_satisfied = 'Yes' if max_dev < 0.15 else 'Moderate'

        val_strs = ' & '.join(f'{v:.0f}' for v in vals)
        lines.append(f'{label} & {val_strs} & {t_satisfied} \\\\')

    lines.append(r'\bottomrule')
    lines.append(r'\end{tabular}')
    lines.append(r'\end{table}')
    return '\n'.join(lines)

def table_latency_at_saturation(exp11_rows, exp31_rows):
    """Table 3: Latency at moderate load (t=32, before saturation)."""
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{Latency at moderate load ($t$=32, RTT = 50\,ms, 95/5 R/W, 50\% weak).}')
    lines.append(r'\label{tab:latency-moderate}')
    lines.append(r'\begin{tabular}{lrrrr}')
    lines.append(r'\toprule')
    lines.append(r'Protocol & Throughput & Strong P50 & Strong P99 & Weak P50 \\')
    lines.append(r'\midrule')

    protocols = [
        ('CURP-HO',         'curpho',        exp31_rows),
        ('CURP-HT',         'curpht',        exp31_rows),
        ('Raft-HT',         'raftht',        exp11_rows),
        ('CURP (baseline)',  'curp-baseline', exp31_rows),
        ('Raft',            'raft',           exp11_rows),
    ]

    for label, proto, rows in protocols:
        filtered = [r for r in rows if r['protocol'] == proto and int(r['threads']) == 32]
        if not filtered:
            continue
        row = filtered[0]
        tput = float(row['throughput'])
        s_p50 = get_val(row, 's_p50')
        s_p99 = get_val(row, 's_p99')
        w_p50 = get_val(row, 'w_p50')

        lines.append(f'{label} & {fmt_k(tput)} & {fmt_ms(s_p50)}\\,ms & {fmt_ms(s_p99)}\\,ms & {fmt_ms(w_p50)}\\,ms \\\\')

    lines.append(r'\bottomrule')
    lines.append(r'\end{tabular}')
    lines.append(r'\end{table}')
    return '\n'.join(lines)

def main():
    base = base_dir()
    exp11_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp1.1.csv')
    exp31_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.1.csv')
    exp32_csv = os.path.join(base, 'results', 'eval-dist-20260307', 'summary-exp3.2.csv')
    out_dir = os.path.join(base, 'plots')

    exp11_rows = load_csv(exp11_csv)
    exp31_rows = load_csv(exp31_csv)
    exp32_rows = load_csv(exp32_csv)

    tables = []
    tables.append('% Auto-generated LaTeX tables for SwiftPaxos evaluation')
    tables.append('% Generated from distributed experiment data (RTT = 50ms)')
    tables.append('')
    tables.append(table_peak_throughput(exp11_rows, exp31_rows))
    tables.append('')
    tables.append(table_t_property(exp32_rows))
    tables.append('')
    tables.append(table_latency_at_saturation(exp11_rows, exp31_rows))
    tables.append('')

    output = '\n'.join(tables)
    print(output)

    out_path = os.path.join(out_dir, 'tables.tex')
    with open(out_path, 'w') as f:
        f.write(output)
    print(f'\nSaved: {out_path}')

if __name__ == '__main__':
    main()
