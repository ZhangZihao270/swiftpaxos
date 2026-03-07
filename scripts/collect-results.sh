#!/bin/bash

# Collect benchmark results from summary.txt files into CSV format.
#
# Usage:
#   ./scripts/collect-results.sh throughput <results-dir> <output.csv>
#     Collect throughput-vs-latency results (Exp 1.1, 2.1, 3.1)
#     Expects: <results-dir>/{protocol}/t{threads}/summary.txt
#
#   ./scripts/collect-results.sh sweep <results-dir> <output.csv>
#     Collect weak ratio sweep results (Exp 3.2)
#     Expects: <results-dir>/{protocol}/w{ratio}/summary.txt

set -e

MODE="$1"
RESULTS_DIR="$2"
OUTPUT_CSV="$3"

if [[ -z "$MODE" || -z "$RESULTS_DIR" || -z "$OUTPUT_CSV" ]]; then
    echo "Usage: $0 {throughput|sweep} <results-dir> <output.csv>"
    exit 1
fi

if [[ ! -d "$RESULTS_DIR" ]]; then
    echo "ERROR: Results directory $RESULTS_DIR not found"
    exit 1
fi

extract_from_summary() {
    local summary="$1"
    if [[ ! -f "$summary" ]]; then
        echo "N/A,N/A,N/A,N/A,N/A,N/A,N/A"
        return
    fi

    python3 - "$summary" << 'PYEOF'
import sys
import re

with open(sys.argv[1]) as f:
    text = f.read()

throughput_m = re.search(r'Aggregate throughput:\s*([\d.]+)', text)
throughput = throughput_m.group(1) if throughput_m else "N/A"

s_lat = re.search(r'Strong Operations.*?Avg: ([\d.]+)ms.*?Avg median: ([\d.]+)ms.*?Max P99: ([\d.]+)ms', text, re.DOTALL)
s_avg = s_lat.group(1) if s_lat else "N/A"
s_p50 = s_lat.group(2) if s_lat else "N/A"
s_p99 = s_lat.group(3) if s_lat else "N/A"

w_lat = re.search(r'Weak Operations.*?Avg: ([\d.]+)ms.*?Avg median: ([\d.]+)ms.*?Max P99: ([\d.]+)ms', text, re.DOTALL)
w_avg = w_lat.group(1) if w_lat else "N/A"
w_p50 = w_lat.group(2) if w_lat else "N/A"
w_p99 = w_lat.group(3) if w_lat else "N/A"

print(f"{throughput},{s_avg},{s_p50},{s_p99},{w_avg},{w_p50},{w_p99}")
PYEOF
}

if [[ "$MODE" == "throughput" ]]; then
    echo "protocol,threads,total_threads,throughput,s_avg,s_p50,s_p99,w_avg,w_p50,w_p99" > "$OUTPUT_CSV"

    # Get number of clients from any summary.txt
    num_clients=3  # default

    for protocol_dir in "$RESULTS_DIR"/*/; do
        protocol=$(basename "$protocol_dir")
        for thread_dir in "$protocol_dir"/t*/; do
            [[ -d "$thread_dir" ]] || continue
            threads=$(basename "$thread_dir" | sed 's/^t//')
            total=$((threads * num_clients))
            summary="$thread_dir/summary.txt"
            metrics=$(extract_from_summary "$summary")
            echo "$protocol,$threads,$total,$metrics" >> "$OUTPUT_CSV"
        done
    done

    # Sort by protocol then threads (numeric)
    header=$(head -1 "$OUTPUT_CSV")
    tail -n +2 "$OUTPUT_CSV" | sort -t, -k1,1 -k2,2n > "${OUTPUT_CSV}.tmp"
    echo "$header" > "$OUTPUT_CSV"
    cat "${OUTPUT_CSV}.tmp" >> "$OUTPUT_CSV"
    rm -f "${OUTPUT_CSV}.tmp"

elif [[ "$MODE" == "sweep" ]]; then
    echo "protocol,weak_ratio,throughput,s_avg,s_p50,s_p99,w_avg,w_p50,w_p99" > "$OUTPUT_CSV"

    for protocol_dir in "$RESULTS_DIR"/*/; do
        protocol=$(basename "$protocol_dir")
        for ratio_dir in "$protocol_dir"/w*/; do
            [[ -d "$ratio_dir" ]] || continue
            ratio=$(basename "$ratio_dir" | sed 's/^w//')
            summary="$ratio_dir/summary.txt"
            metrics=$(extract_from_summary "$summary")
            echo "$protocol,$ratio,$metrics" >> "$OUTPUT_CSV"
        done
    done

    # Sort by protocol then ratio (numeric)
    header=$(head -1 "$OUTPUT_CSV")
    tail -n +2 "$OUTPUT_CSV" | sort -t, -k1,1 -k2,2n > "${OUTPUT_CSV}.tmp"
    echo "$header" > "$OUTPUT_CSV"
    cat "${OUTPUT_CSV}.tmp" >> "$OUTPUT_CSV"
    rm -f "${OUTPUT_CSV}.tmp"

else
    echo "ERROR: Unknown mode '$MODE'. Use 'throughput' or 'sweep'."
    exit 1
fi

echo "Results written to $OUTPUT_CSV"
echo ""
column -t -s, "$OUTPUT_CSV"
