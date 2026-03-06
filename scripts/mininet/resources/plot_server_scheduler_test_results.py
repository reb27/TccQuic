#!/usr/bin/env python3

import csv
import os
import sys
import datetime as dt
import matplotlib.pyplot as plt


CLASS_NAMES = {0: 'high', 1: 'medium', 2: 'low'}
CLASS_ORDER = ['high', 'medium', 'low']
CLASS_COLORS = {
    'high': 'tab:red',
    'medium': 'tab:orange',
    'low': 'tab:blue',
}


def _bool(v: str) -> bool:
    return str(v).strip().lower() in ('1', 'true', 'yes')


def _read_csv(path: str):
    try:
        with open(path, newline='') as f:
            return list(csv.DictReader(f))
    except FileNotFoundError:
        return None


def _rel_seconds_from_iso(ts_list):
    times = []
    for ts in ts_list:
        try:
            times.append(dt.datetime.fromisoformat(ts))
        except Exception:
            pass
    if not times:
        return [], None
    t0 = min(times)
    return [(t - t0).total_seconds() for t in times], t0


def _rel_seconds_from_ns(ns_list):
    try:
        ns_list = [int(n) for n in ns_list]
    except Exception:
        return [], None
    if not ns_list:
        return [], None
    t0 = min(ns_list)
    return [((n - t0) / 1e9) for n in ns_list], t0


# -------------------- Client plot --------------------
def render_client_plot(input_files, output_dir):
    # Prefer aggregated client summary if present
    summary_files = [p for p in input_files if os.path.basename(p).startswith('statistics-summary-')]
    metrics = {}
    if summary_files:
        rows = _read_csv(summary_files[0]) or []
        if rows:
            row = rows[0]
            if 'segment_completion_rate_percent' in row:
                try:
                    metrics['segment_completion_rate_percent'] = float(row['segment_completion_rate_percent'])
                except Exception:
                    pass
            if 'segment_completion_rate_fov_percent' in row:
                try:
                    metrics['segment_completion_rate_fov_percent'] = float(row['segment_completion_rate_fov_percent'])
                except Exception:
                    pass
            if 'stale_bytes_ratio_percent' in row:
                try:
                    metrics['stale_bytes_ratio_percent'] = float(row['stale_bytes_ratio_percent'])
                except Exception:
                    pass
    else:
        # Fallback: compute completion rates from statistics-*.csv
        stats_files = [p for p in input_files if os.path.basename(p).startswith('statistics-') and 'summary' not in os.path.basename(p)]
        ok_total = 0
        all_total = 0
        ok_fov = 0
        fov_total = 0
        for path in stats_files:
            rows = _read_csv(path) or []
            for r in rows:
                all_total += 1
                if _bool(r.get('ok', 'false')):
                    ok_total += 1
                if _bool(r.get('in_fov', 'false')):
                    fov_total += 1
                    if _bool(r.get('ok', 'false')):
                        ok_fov += 1
        if all_total > 0:
            metrics['segment_completion_rate_percent'] = 100.0 * ok_total / all_total
        if fov_total > 0:
            metrics['segment_completion_rate_fov_percent'] = 100.0 * ok_fov / fov_total
        # stale_bytes_ratio_percent not available from client fallback

    # If nothing to plot, return silently
    if not metrics:
        print('[warn] No client metrics found to plot')
        return

    labels = []
    values = []
    for key in (
        'segment_completion_rate_percent',
        'segment_completion_rate_fov_percent',
        'stale_bytes_ratio_percent',
    ):
        if key in metrics:
            labels.append(key.replace('_', '\n'))
            values.append(metrics[key])

    plt.figure(figsize=(6, 4))
    bars = plt.bar(range(len(values)), values, color='tab:blue')
    plt.ylim(0, 100)
    plt.xticks(range(len(labels)), labels)
    plt.ylabel('percent')
    plt.title('Client Summary')
    for b, v in zip(bars, values):
        plt.text(b.get_x() + b.get_width() / 2, v + 1, f'{v:.1f}%', ha='center', va='bottom', fontsize=8)
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'client.png'))
    plt.close()


# -------------------- Server plot helpers --------------------
def _plot_server_queue_len(ql_path, output_dir):
    rows = _read_csv(ql_path) or []
    if not rows:
        print('[warn] queue_len.csv is empty')
        return
    by_class = {c: {'ts': [], 'len': []} for c in CLASS_ORDER}
    for r in rows:
        c = r.get('class')
        if c in by_class:
            by_class[c]['ts'].append(r.get('ts'))
            try:
                by_class[c]['len'].append(int(float(r.get('queue_len', '0'))))
            except Exception:
                by_class[c]['len'].append(0)

    all_ts = []
    for c in CLASS_ORDER:
        all_ts.extend(by_class[c]['ts'])
    rel_all, t0 = _rel_seconds_from_iso(all_ts)
    if t0 is None:
        print('[warn] Could not parse queue_len timestamps')
        return
    ts_map = {iso: rel for iso, rel in zip(all_ts, rel_all)}

    plt.figure(figsize=(7, 4))
    drew = False
    for c in CLASS_ORDER:
        if by_class[c]['ts']:
            x = [ts_map[ts] for ts in by_class[c]['ts'] if ts in ts_map]
            y = by_class[c]['len']
            if x and y:
                plt.plot(x, y, label=c, color=CLASS_COLORS.get(c))
                drew = True
    if not drew:
        plt.close()
        print('[warn] No queue_len series to draw')
        return
    plt.xlabel('time (s)')
    plt.ylabel('queue length (pkts)')
    plt.legend()
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_queue.png'))
    plt.close()


def _plot_server_rsp_scatter(rl_path, output_dir):
    # Deprecated: scatter plot of response time vs time was removed
    return


def _plot_server_rsp_cdf(rl_path, output_dir):
    rows = _read_csv(rl_path) or []
    if not rows:
        return
    per_class = {c: [] for c in CLASS_ORDER}
    for r in rows:
        try:
            c = CLASS_NAMES.get(int(r['class']), str(r['class']))
            rsp = float(r['rsp_ms'])
        except Exception:
            continue
        if c in per_class:
            per_class[c].append(rsp)

    plt.figure(figsize=(7, 4))
    drew = False
    for c in CLASS_ORDER:
        xs = sorted(per_class[c])
        if not xs:
            continue
        n = len(xs)
        ys = [(i + 1) / n for i in range(n)]
        plt.plot(xs, ys, label=c, color=CLASS_COLORS.get(c))
        drew = True
    if not drew:
        plt.close()
        return
    plt.xlabel('response time (ms)')
    plt.ylabel('CDF')
    plt.legend()
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_rsp_cdf.png'))
    plt.close()


def _plot_server_drop_rate(rl_path, output_dir):
    rows = _read_csv(rl_path) or []
    if not rows:
        return
    counts = {c: {'tot': 0, 'drop': 0} for c in CLASS_ORDER}
    for r in rows:
        try:
            c = CLASS_NAMES.get(int(r['class']), str(r['class']))
        except Exception:
            continue
        if c not in counts:
            continue
        counts[c]['tot'] += 1
        if _bool(r.get('drop', 'false')):
            counts[c]['drop'] += 1

    labels = []
    values = []
    for c in CLASS_ORDER:
        tot = counts[c]['tot']
        if tot == 0:
            continue
        dr = 100.0 * counts[c]['drop'] / tot
        labels.append(c)
        values.append(dr)

    if not values:
        return
    plt.figure(figsize=(5, 4))
    bars = plt.bar(range(len(values)), values, color=[CLASS_COLORS[c] for c in labels])
    plt.xticks(range(len(labels)), labels)
    plt.ylabel('drop rate (%)')
    plt.title('Deadline drops per class')
    for b, v in zip(bars, values):
        plt.text(b.get_x() + b.get_width() / 2, v + 0.5, f'{v:.1f}%', ha='center', va='bottom', fontsize=8)
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_drop_rate.png'))
    plt.close()


def _plot_server_summary(ss_path, output_dir):
    rows = _read_csv(ss_path) or []
    if not rows:
        return
    row = rows[-1]

    labels = []
    values = []
    key_map = {
        'high': 'throughput_high_kbps',
        'medium': 'throughput_med_kbps',
        'low': 'throughput_low_kbps',
    }
    for c in CLASS_ORDER:
        key = key_map.get(c)
        if key in row:
            try:
                values.append(float(row[key]))
                labels.append(c)
            except Exception:
                pass

    if not values:
        return
    plt.figure(figsize=(5, 4))
    bars = plt.bar(range(len(values)), values, color=[CLASS_COLORS[c] for c in labels])
    plt.xticks(range(len(labels)), labels)
    plt.ylabel('throughput (kbps)')
    plt.title('Server throughput per class')
    for b, v in zip(bars, values):
        plt.text(b.get_x() + b.get_width() / 2, v + max(values) * 0.01, f'{v:.1f}', ha='center', va='bottom', fontsize=8)
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_summary.png'))
    plt.close()


def _plot_server_class_share(ss_path, output_dir):
    rows = _read_csv(ss_path) or []
    if not rows:
        return
    row = rows[-1]

    labels = []
    values = []
    key_map = {
        'high': 'class_share_high_pct',
        'medium': 'class_share_med_pct',
        'low': 'class_share_low_pct',
    }
    for c in CLASS_ORDER:
        key = key_map.get(c)
        if key in row:
            try:
                values.append(float(row[key]))
                labels.append(c)
            except Exception:
                pass

    if not values:
        return
    plt.figure(figsize=(5, 4))
    bars = plt.bar(range(len(values)), values, color=[CLASS_COLORS[c] for c in labels])
    plt.xticks(range(len(labels)), labels)
    plt.ylabel('class share (%)')
    plt.title('Traffic share per class')
    for b, v in zip(bars, values):
        plt.text(b.get_x() + b.get_width() / 2, v + max(values) * 0.01, f'{v:.1f}%', ha='center', va='bottom', fontsize=8)
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_class_share.png'))
    plt.close()


def _plot_server_work_conserving(wc_path, output_dir):
    rows = _read_csv(wc_path) or []
    if not rows:
        return
    ts = []
    ratio = []
    for r in rows:
        try:
            ts.append(r.get('ts'))
            ratio.append(float(r.get('ratio', '0')))
        except Exception:
            continue
    rel_t, t0 = _rel_seconds_from_iso(ts)
    if t0 is None or not rel_t or not ratio:
        return
    plt.figure(figsize=(7, 4))
    plt.plot(rel_t, ratio, color='tab:green')
    plt.xlabel('time (s)')
    plt.ylabel('busy ratio (0-1)')
    plt.title('Work-conserving ratio over time')
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_work_conserving.png'))
    plt.close()


def _plot_server_backlog(wc_path, output_dir):
    rows = _read_csv(wc_path) or []
    if not rows:
        return
    ts = []
    backlog_ms = []
    for r in rows:
        try:
            ts.append(r.get('ts'))
            backlog_ms.append(float(r.get('backlog_ms', '0')))
        except Exception:
            continue
    rel_t, t0 = _rel_seconds_from_iso(ts)
    if t0 is None or not rel_t or not backlog_ms:
        return
    plt.figure(figsize=(7, 4))
    plt.plot(rel_t, backlog_ms, color='tab:purple')
    plt.xlabel('time (s)')
    plt.ylabel('backlog_ms (per window)')
    plt.title('Backlog time while queue>0')
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_backlog_ms.png'))
    plt.close()


def _plot_server_avg_rsp_from_class_agg(ca_path, output_dir):
    rows = _read_csv(ca_path) or []
    if not rows:
        return

    series = {c: {'t': [], 'v': []} for c in CLASS_ORDER}
    for r in rows:
        try:
            ts = r.get('ts')
            c = r.get('class')
            if c not in series or not ts:
                continue
            # avg_response_time_ms já é média acumulada até aquele ponto
            v = float(r.get('avg_response_time_ms', '0'))
            series[c]['t'].append(ts)
            series[c]['v'].append(v)
        except Exception:
            continue

    all_ts = []
    for c in CLASS_ORDER:
        all_ts.extend(series[c]['t'])
    rel_all, t0 = _rel_seconds_from_iso(all_ts)
    if t0 is None or not rel_all:
        return
    ts_map = {iso: rel for iso, rel in zip(all_ts, rel_all)}

    plt.figure(figsize=(7, 4))
    drew = False
    for c in CLASS_ORDER:
        if not series[c]['t']:
            continue
        x = [ts_map[ts] for ts in series[c]['t'] if ts in ts_map]
        y = series[c]['v']
        if x and y:
            plt.plot(x, y, label=c, color=CLASS_COLORS.get(c))
            drew = True
    if not drew:
        plt.close()
        return
    plt.xlabel('time (s)')
    plt.ylabel('avg response time (ms)')
    plt.title('Avg response time per class (class_agg)')
    plt.legend()
    os.makedirs(output_dir, exist_ok=True)
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'server_avg_rsp_class_agg.png'))
    plt.close()


# -------------------- Server plot (orchestrator) --------------------
def render_server_plot(input_files, output_dir):
    ql_files = [p for p in input_files if os.path.basename(p) == 'queue_len.csv']
    rl_files = [p for p in input_files if os.path.basename(p) == 'reqlog.csv']
    ss_files = [p for p in input_files if os.path.basename(p) == 'server_summary.csv']
    wc_files = [p for p in input_files if os.path.basename(p) == 'work_conserving.csv']
    ca_files = [p for p in input_files if os.path.basename(p) == 'class_agg.csv']

    if not (ql_files or rl_files or ss_files or wc_files or ca_files):
        print('[warn] No server inputs found (queue_len.csv, reqlog.csv, server_summary.csv, work_conserving.csv or class_agg.csv)')
        return

    if ql_files:
        _plot_server_queue_len(ql_files[0], output_dir)

    if rl_files:
        _plot_server_rsp_cdf(rl_files[0], output_dir)
        _plot_server_drop_rate(rl_files[0], output_dir)

    if ss_files:
        _plot_server_summary(ss_files[0], output_dir)
        _plot_server_class_share(ss_files[0], output_dir)

    if wc_files:
        _plot_server_work_conserving(wc_files[0], output_dir)
        _plot_server_backlog(wc_files[0], output_dir)

    if ca_files:
        _plot_server_avg_rsp_from_class_agg(ca_files[0], output_dir)


def main():
    if len(sys.argv) < 2:
        print('Usage A (list of CSVs): plot_server_scheduler_test_results.py <INPUT1.csv> [INPUT2.csv ...] <OUTPUT_DIR>')
        print('Usage B (dirs):         plot_server_scheduler_test_results.py <INPUT_DIR> <OUTPUT_DIR>')
        sys.exit(1)

    args = sys.argv[1:]
    # If last arg is a directory, treat it as output
    if os.path.isdir(args[-1]):
        output_dir = args[-1]
        inputs = args[:-1]
        # If inputs are empty but first is a directory, use its CSVs
        input_files = []
        if len(inputs) == 1 and os.path.isdir(inputs[0]):
            in_dir = inputs[0]
            try:
                for name in os.listdir(in_dir):
                    if name.lower().endswith('.csv'):
                        input_files.append(os.path.join(in_dir, name))
            except Exception:
                pass
        else:
            # Filter only existing CSV files
            for p in inputs:
                if os.path.isfile(p) and p.lower().endswith('.csv'):
                    input_files.append(p)
    else:
        # If last arg is not a directory, assume the classic mode <CSV> <OUTPUT_DIR>
        if len(args) != 2:
            print('error: expected <CSV> <OUTPUT_DIR> or <CSV...> <OUTPUT_DIR>')
            sys.exit(1)
        input_files = [args[0]] if os.path.isfile(args[0]) else []
        output_dir = args[1]

    # De-duplicate keeping order
    seen = set()
    ordered_inputs = []
    for p in input_files:
        if p not in seen:
            ordered_inputs.append(p)
            seen.add(p)

    # Render the two simple plots
    print('Rendering client plot...')
    render_client_plot(ordered_inputs, output_dir)
    print('Rendering server plot...')
    render_server_plot(ordered_inputs, output_dir)


if __name__ == '__main__':
    main()
