#!/usr/bin/env python3

import matplotlib.pyplot as plt
import csv
import sys

INPUT_FILE = sys.argv[1]
OUTPUT_DIR = sys.argv[2]

input = []
with open(INPUT_FILE) as f:
    input = [{
        'time': float(row['time_ns']) / 1000000,
        'segment': int(row['segment']),
        'tile': int(row['tile']),
        'priority': int(row['priority']),
        'latency': float(row['latency_ns']) / 1000000,
        'skipped': row['skipped'] == 'true',
        'timedout': row['timedout'] == 'true',
    } for row in csv.DictReader(f)]

min_segment = 0
include_priority = [False, False, False]
for row in input:
    if row['segment'] < min_segment:
        min_segment = row['segment']
    include_priority[row['priority']] = True

def plot_latencies():
    plot_x = [[], [], []]
    plot_y = [[], [], []]

    for row in input:
        plot_x[row['priority']].append(row['time'] / 1000)
        plot_y[row['priority']].append(row['latency'])

    if include_priority[0]:
        plt.scatter(plot_x[0], plot_y[0], s=2, label='high priority')
    if include_priority[1]:
        plt.scatter(plot_x[1], plot_y[1], s=2, label='medium priority')
    if include_priority[2]:
        plt.scatter(plot_x[2], plot_y[2], s=2, label='low priority')
    plt.legend()
    plt.xlabel('Send time (s)')
    plt.ylabel('Latency (ms)')

    plt.savefig(OUTPUT_DIR + '/plot-latency-linear.png')

    plt.yscale('log')
    plt.savefig(OUTPUT_DIR + '/plot-latency-log.png')
    plt.close()

def get_missing_rate():
    total = [0, 0, 0]
    skipped = [0, 0, 0]
    timedout = [0, 0, 0]
    for row in input:
        total[row['priority']] += 1
        if row['skipped']:
            skipped[row['priority']] += 1
        if row['timedout']:
            timedout[row['priority']] += 1

    with open(OUTPUT_DIR + '/missing-rate.txt', 'w') as f:
        for priority, priority_text in enumerate(['High', 'Medium', 'Low']):
            if total[priority] != 0:
                f.write('%s priority:\n' % priority_text)
                f.write('Total: %d\n' % total[priority])
                f.write('Skipped: %d\t(%.2f %%)\n' % (skipped[priority], 100 * skipped[priority] / total[priority]))
                f.write('Missing: %d\t(%.2f %%)\n' % (timedout[priority], 100 * timedout[priority] / total[priority]))
                f.write('\n')

print('Plotting latencies')
plot_latencies()
print('Getting missing rate')
get_missing_rate()
