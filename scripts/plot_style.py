"""
Shared plotting style for SwiftPaxos experiment figures.
Uses Wong's colorblind-safe palette and consistent typography.
"""

import csv
import os
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

# ── Wong colorblind-safe palette ──────────────────────────────────────
# From: https://www.nature.com/articles/nmeth.1618
WONG = {
    'blue':    '#0072B2',
    'orange':  '#E69F00',
    'green':   '#009E73',
    'red':     '#D55E00',
    'purple':  '#CC79A7',
    'cyan':    '#56B4E9',
    'yellow':  '#F0E442',
    'black':   '#000000',
}

# Protocol colors — distinct even for colorblind viewers
PROTOCOL_COLORS = {
    'raftht':        WONG['red'],
    'raft':          WONG['orange'],
    'curpho':        WONG['blue'],
    'curpht':        WONG['green'],
    'curp-baseline': WONG['purple'],
    'epaxos':        WONG['cyan'],
    # Exp 1.1 extra protocols (don't appear in the same figure as curp/epaxos)
    'mongotunable':  WONG['black'],
    'pileus':        WONG['purple'],
    'pileusht':      WONG['cyan'],
    # Exp 2.1
    'epaxosho':      WONG['blue'],
}

PROTOCOL_MARKERS = {
    'raftht':        's',
    'raft':          '^',
    'curpho':        'o',
    'curpht':        'D',
    'curp-baseline': 'v',
    'epaxos':        'P',
    'mongotunable':  '*',
    'pileus':        'P',
    'pileusht':      'D',
    'epaxosho':      'o',
}

PROTOCOL_LABELS = {
    'raftht':        'Raft-HT',
    'raft':          'Raft',
    'curpho':        'CURP-HO',
    'curpht':        'CURP-HT',
    'curp-baseline': 'CURP (baseline)',
    'epaxos':        'EPaxos',
    'mongotunable':  'MongoDB',
    'pileus':        'Pileus',
    'pileusht':      'Pileus-HT',
    'epaxosho':      'EPaxos-HO',
}

def load_csv_optional(path):
    """Load CSV if file exists, return empty list otherwise."""
    if not os.path.exists(path):
        return []
    return load_csv(path)

def setup_style():
    plt.rcParams.update({
        'font.family': 'serif',
        'font.size': 11,
        'axes.labelsize': 13,
        'axes.titlesize': 13,
        'legend.fontsize': 9,
        'xtick.labelsize': 10,
        'ytick.labelsize': 10,
        'figure.dpi': 300,
        'savefig.dpi': 300,
        'savefig.bbox': 'tight',
        'savefig.pad_inches': 0.1,
        'lines.linewidth': 2.0,
        'lines.markersize': 7,
        'axes.grid': True,
        'grid.alpha': 0.3,
        'grid.linestyle': '--',
    })

def load_csv(path):
    rows = []
    with open(path, newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)
    return rows

def get_val(row, key):
    """Get a float value from a CSV row, returning None for N/A."""
    v = row.get(key, 'N/A')
    if v == 'N/A' or v is None or v == '':
        return None
    return float(v)

def extract_tput_latency(rows, protocol):
    """Extract throughput-vs-latency data sorted by thread count."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['threads']))
    return {
        'threads': [int(r['threads']) for r in filtered],
        'throughput': [float(r['throughput']) for r in filtered],
        's_p50': [get_val(r, 's_p50') for r in filtered],
        's_p99': [get_val(r, 's_p99') for r in filtered],
        'w_p50': [get_val(r, 'w_p50') for r in filtered],
        'w_p99': [get_val(r, 'w_p99') for r in filtered],
    }

def clean_pairs(xs, ys):
    """Filter out None/zero entries from paired lists."""
    xc, yc = [], []
    for x, y in zip(xs, ys):
        if x is not None and y is not None and x > 0:
            xc.append(x)
            yc.append(y)
    return xc, yc

def pareto_frontier(xs, ys):
    """Keep only points up to and including peak throughput.

    For throughput-vs-latency curves, after peak throughput adding more
    threads degrades both throughput and latency, creating visual loops.
    This trims to the Pareto-optimal prefix.
    """
    if not xs:
        return xs, ys
    peak_idx = max(range(len(xs)), key=lambda i: xs[i])
    return xs[:peak_idx + 1], ys[:peak_idx + 1]

def kops_formatter(x, _):
    """Format throughput axis as Kops/sec."""
    return f'{x/1000:.0f}'

def load_multi_run_csv(paths):
    """Load multiple CSV files and aggregate by (protocol, threads).

    Returns rows with median values and min/max for error bars.
    Each row dict has keys like 'throughput', 'throughput_lo', 'throughput_hi'.
    """
    import statistics
    all_runs = [load_csv(p) for p in paths if os.path.exists(p)]
    if not all_runs:
        return []

    groups = {}
    for run_rows in all_runs:
        for row in run_rows:
            key = (row['protocol'], row['threads'])
            groups.setdefault(key, []).append(row)

    agg_rows = []
    numeric_keys = ['throughput', 's_avg', 's_p50', 's_p99', 'w_avg', 'w_p50', 'w_p99']
    for (proto, threads), rows in sorted(groups.items(), key=lambda x: (x[0][0], int(x[0][1]))):
        agg = {'protocol': proto, 'threads': threads, 'total_threads': rows[0].get('total_threads', '')}
        for k in numeric_keys:
            vals = []
            for r in rows:
                v = r.get(k, 'N/A')
                if v != 'N/A' and v is not None and v != '':
                    vals.append(float(v))
            if vals:
                med = statistics.median(vals)
                agg[k] = str(med)
                agg[f'{k}_lo'] = str(min(vals))
                agg[f'{k}_hi'] = str(max(vals))
            else:
                agg[k] = 'N/A'
                agg[f'{k}_lo'] = 'N/A'
                agg[f'{k}_hi'] = 'N/A'
        agg_rows.append(agg)
    return agg_rows

def extract_tput_latency_with_errbars(rows, protocol):
    """Extract throughput-vs-latency data with min/max error bars."""
    filtered = [r for r in rows if r['protocol'] == protocol]
    filtered.sort(key=lambda r: int(r['threads']))
    result = {
        'threads': [int(r['threads']) for r in filtered],
        'throughput': [float(r['throughput']) for r in filtered],
        's_p50': [get_val(r, 's_p50') for r in filtered],
        's_p99': [get_val(r, 's_p99') for r in filtered],
        'w_p50': [get_val(r, 'w_p50') for r in filtered],
        'w_p99': [get_val(r, 'w_p99') for r in filtered],
    }
    for key in ['throughput', 's_p50', 's_p99', 'w_p50', 'w_p99']:
        result[f'{key}_lo'] = [get_val(r, f'{key}_lo') for r in filtered]
        result[f'{key}_hi'] = [get_val(r, f'{key}_hi') for r in filtered]
    return result

def base_dir():
    return os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

def extract_tput_latency_wg(rows, protocol, write_group):
    """Extract throughput-vs-latency from the newer CSV format.

    Newer CSVs have columns: write_group, protocol, threads,
    avg_throughput, avg_s_p50, avg_s_p99, avg_w_p50, avg_w_p99.
    write_group is an integer (5 or 50).
    """
    filtered = [r for r in rows
                if r['protocol'] == protocol
                and int(r['write_group']) == write_group]
    filtered.sort(key=lambda r: int(r['threads']))
    return {
        'threads':    [int(r['threads']) for r in filtered],
        'throughput': [float(r['avg_throughput']) for r in filtered],
        's_p50':      [get_val(r, 'avg_s_p50') for r in filtered],
        's_p99':      [get_val(r, 'avg_s_p99') for r in filtered],
        'w_p50':      [get_val(r, 'avg_w_p50') for r in filtered],
        'w_p99':      [get_val(r, 'avg_w_p99') for r in filtered],
    }

def merge_rows(*row_lists):
    """Merge multiple row lists (concatenate)."""
    result = []
    for lst in row_lists:
        result.extend(lst)
    return result

def save_figure(fig, out_dir, name):
    os.makedirs(out_dir, exist_ok=True)
    for ext in ['pdf', 'png']:
        path = os.path.join(out_dir, f'{name}.{ext}')
        fig.savefig(path)
        print(f'Saved: {path}')
    plt.close(fig)
