#!/usr/bin/env python3

import matplotlib.pyplot as plt
import csv
import sys

INPUT = sys.argv[1]
OUTPUT = sys.argv[2]

plot_x = [[], [], []]
plot_y = [[], [], []]

with open(INPUT) as csv_file:
    for row in csv.DictReader(csv_file):
        plot_x[int(row['priority'])].append(float(row['time_ns'])    / 1000000000)
        plot_y[int(row['priority'])].append(float(row['latency_ns']) / 1000000)

plt.scatter(plot_x[0], plot_y[0], s=2, label='high priority')
plt.scatter(plot_x[1], plot_y[1], s=2, label='medium priority')
plt.scatter(plot_x[2], plot_y[2], s=2, label='low priority')
plt.legend()
plt.xlabel('Send time (s)')
plt.ylabel('Latency (ms)')
plt.savefig(OUTPUT + '/plot.png')
