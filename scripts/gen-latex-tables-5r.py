#!/usr/bin/env python3
"""
Generate LaTeX tables from 5-replica experiment CSV data.
Uses Phase 78 data (multi-run median for Exp 3.1, Phase 78.3b for Exp 3.2).
Outputs to evaluation/plots/tables-5r.tex (and prints to stdout).
"""

import csv
import json
import os
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from plot_style import load_csv, load_multi_run_csv, get_val, base_dir

def fmt_k(v):
    if v >= 1000:
        return f'{v/1000:.1f}K'
    return f'{v:.0f}'

def fmt_ms(v):
    if v is None:
        return '---'
    if v < 1:
        return f'{v:.2f}'
    return f'{v:.0f}'

def get_peak(rows, protocol):
    filtered = [r for r in rows if r['protocol'] == protocol]
    if not filtered:
        return None
    return max(filtered, key=lambda r: float(r['throughput']))

def percentile(vals, p):
    if not vals:
        return None
    idx = min(int(len(vals) * p / 100), len(vals) - 1)
    return vals[idx]

def table_peak_throughput(exp11_rows, exp31_rows):
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{Peak throughput comparison (5 replicas, RTT = 50\,ms, 95/5 R/W, 50\% weak, Zipfian). Exp 3.1 values are median of 3 runs.}')
    lines.append(r'\label{tab:peak-throughput-5r}')
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
        if row is None:
            continue
        tput = float(row['throughput'])
        threads = int(row['threads'])
        s_p50 = get_val(row, 's_p50')
        w_p50 = get_val(row, 'w_p50')
        speedup = tput / baseline_peak

        if proto == 'raft':
            speedup_str = '---'
        elif proto == 'curp-baseline':
            speedup_str = '1.0$\\times$'
        else:
            speedup_str = f'{speedup:.1f}$\\times$'

        lines.append(f'{label} & {fmt_k(tput)} & {threads}$\\times$5 & {fmt_ms(s_p50)} & {fmt_ms(w_p50)} & {speedup_str} \\\\')

    lines.append(r'\bottomrule')
    lines.append(r'\end{tabular}')
    lines.append(r'\end{table}')
    return '\n'.join(lines)

def table_t_property(exp32_rows):
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{Strong P50 latency across weak ratios (5 replicas, RTT = 50\,ms, 95/5 R/W, $t$=32).}')
    lines.append(r'\label{tab:t-property-5r}')
    lines.append(r'\begin{tabular}{lrrrrrrl}')
    lines.append(r'\toprule')
    lines.append(r'Protocol & $w$=0\% & $w$=10\% & $w$=25\% & $w$=50\% & $w$=75\% & $w$=100\% & T satisfied? \\')
    lines.append(r'\midrule')

    for proto, label in [('curp-baseline', 'CURP (baseline)'), ('curpht', 'CURP-HT'), ('curpho', 'CURP-HO')]:
        filtered = [r for r in exp32_rows if r['protocol'] == proto]
        filtered.sort(key=lambda r: int(r['weak_ratio']))
        vals = [float(r['s_p50']) for r in filtered]
        # Exclude w100 from T property check (no strong ops at w100)
        check_vals = vals[:5]  # w0, w10, w25, w50, w75
        mean_val = sum(check_vals) / len(check_vals)
        max_dev = max(abs(v - mean_val) / mean_val for v in check_vals)
        t_satisfied = 'Yes' if max_dev < 0.15 else 'Moderate'

        val_strs = ' & '.join(f'{v:.0f}' for v in vals)
        lines.append(f'{label} & {val_strs} & {t_satisfied} \\\\')

    lines.append(r'\bottomrule')
    lines.append(r'\end{tabular}')
    lines.append(r'\end{table}')
    return '\n'.join(lines)

def table_latency_at_moderate(exp11_rows, exp31_rows):
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{Latency at moderate load ($t$=32, 5 replicas, RTT = 50\,ms, 95/5 R/W, 50\% weak). Exp 3.1 values are median of 3 runs.}')
    lines.append(r'\label{tab:latency-moderate-5r}')
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

def load_cdf_latencies_5r(base):
    # Phase 78 run1 for CDF latencies
    results_dir = os.path.join(base, 'results', 'eval-5r-phase78-run1')
    exp11_dir = os.path.join(base, 'results', 'eval-5r-20260308')
    data = {}
    for proto, exp_base in [
        ('curpho', results_dir), ('curpht', results_dir), ('curp-baseline', results_dir),
        ('raftht', exp11_dir), ('raft', exp11_dir),
    ]:
        exp = 'exp3.1' if proto in ('curpho', 'curpht', 'curp-baseline') else 'exp1.1'
        path = os.path.join(exp_base, exp, proto, 't32', 'latencies.json')
        if os.path.exists(path):
            with open(path) as f:
                data[proto] = json.load(f)
    return data

def table_cdf_percentiles(cdf_data):
    if not cdf_data:
        return ''
    lines = []
    lines.append(r'\begin{table}[t]')
    lines.append(r'\centering')
    lines.append(r'\caption{Latency percentiles at moderate load ($t$=32, 5 replicas, RTT = 50\,ms, 95/5 R/W, 50\% weak).}')
    lines.append(r'\label{tab:cdf-percentiles-5r}')
    lines.append(r'\begin{tabular}{llrrrrrr}')
    lines.append(r'\toprule')
    lines.append(r'Protocol & Type & P1 & P25 & P50 & P75 & P99 & P99.9 \\')
    lines.append(r'\midrule')

    protocols = [
        ('CURP-HO', 'curpho'), ('CURP-HT', 'curpht'),
        ('Raft-HT', 'raftht'), ('CURP (baseline)', 'curp-baseline'),
        ('Raft', 'raft'),
    ]

    last_proto = [p for _, p in protocols if p in cdf_data][-1] if cdf_data else None

    for label, proto in protocols:
        if proto not in cdf_data:
            continue
        lat = cdf_data[proto]
        s_vals = sorted(lat.get('strong_write', []) + lat.get('strong_read', []))
        if s_vals:
            ps = [percentile(s_vals, p) for p in [1, 25, 50, 75, 99, 99.9]]
            ps_str = ' & '.join(fmt_ms(v) for v in ps)
            lines.append(f'{label} & Strong & {ps_str} \\\\')

        w_vals = sorted(lat.get('weak_write', []) + lat.get('weak_read', []))
        if w_vals:
            ps = [percentile(w_vals, p) for p in [1, 25, 50, 75, 99, 99.9]]
            ps_str = ' & '.join(fmt_ms(v) for v in ps)
            lines.append(f' & Weak & {ps_str} \\\\')

        lines.append(r'\midrule' if proto != last_proto else r'\bottomrule')

    if lines[-1] == r'\midrule':
        lines[-1] = r'\bottomrule'

    lines.append(r'\end{tabular}')
    lines.append(r'\end{table}')
    return '\n'.join(lines)

def export_cdf_summary_csv(cdf_data, out_path):
    if not cdf_data:
        return
    rows = []
    protocols = [
        ('curpho', 'CURP-HO'), ('curpht', 'CURP-HT'),
        ('raftht', 'Raft-HT'), ('curp-baseline', 'CURP (baseline)'),
        ('raft', 'Raft'),
    ]
    pcts = [1, 5, 10, 25, 50, 75, 90, 95, 99, 99.9]

    for proto, label in protocols:
        if proto not in cdf_data:
            continue
        lat = cdf_data[proto]
        for op_type in ['strong_read', 'strong_write', 'weak_read', 'weak_write']:
            vals = lat.get(op_type, [])
            if not vals:
                continue
            row = {'protocol': label, 'op_type': op_type, 'count': len(vals)}
            for p in pcts:
                row[f'p{p}'] = f'{percentile(vals, p):.2f}'
            rows.append(row)

    if rows:
        fieldnames = ['protocol', 'op_type', 'count'] + [f'p{p}' for p in pcts]
        with open(out_path, 'w', newline='') as f:
            writer = csv.DictWriter(f, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(rows)
        print(f'Saved: {out_path}')

def main():
    base = base_dir()

    # Phase 78 multi-run median for Exp 3.1
    exp31_csv_paths = [
        os.path.join(base, 'results', f'eval-5r-phase78-run{i}', 'summary-exp3.1.csv')
        for i in range(1, 4)
    ]
    exp31_rows = load_multi_run_csv(exp31_csv_paths)

    # Phase 78.3b for Exp 3.2
    exp32_csv = os.path.join(base, 'results', 'eval-5r-exp3.2-phase78-20260309', 'summary-exp3.2.csv')
    exp32_rows = load_csv(exp32_csv)

    # Old data for Exp 1.1 (raftht/raft — not re-run in Phase 78)
    exp11_csv = os.path.join(base, 'results', 'eval-5r-20260308', 'summary-exp1.1.csv')
    exp11_rows = load_csv(exp11_csv) if os.path.exists(exp11_csv) else []

    out_dir = os.path.join(base, 'evaluation', 'plots')

    cdf_data = load_cdf_latencies_5r(base)
    if cdf_data:
        print(f'CDF data loaded: {", ".join(cdf_data.keys())}')

    tables = []
    tables.append('% Auto-generated LaTeX tables for SwiftPaxos 5-replica evaluation')
    tables.append('% Phase 78 data: Exp 3.1 median of 3 runs, Exp 3.2 phase78.3b (t=32)')
    tables.append('')
    tables.append(table_peak_throughput(exp11_rows, exp31_rows))
    tables.append('')
    tables.append(table_t_property(exp32_rows))
    tables.append('')
    tables.append(table_latency_at_moderate(exp11_rows, exp31_rows))

    if cdf_data:
        tables.append('')
        tables.append(table_cdf_percentiles(cdf_data))

    tables.append('')

    output = '\n'.join(tables)
    print(output)

    out_path = os.path.join(out_dir, 'tables-5r.tex')
    with open(out_path, 'w') as f:
        f.write(output)
    print(f'\nSaved: {out_path}')

    if cdf_data:
        csv_path = os.path.join(out_dir, 'cdf-5r-summary.csv')
        export_cdf_summary_csv(cdf_data, csv_path)

if __name__ == '__main__':
    main()
