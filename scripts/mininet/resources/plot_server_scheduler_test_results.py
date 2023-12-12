#!/usr/bin/env python3

import matplotlib.pyplot as plt
import csv
import sys

INPUT = sys.argv[1]
OUTPUT = sys.argv[2]

plot_x0 = []
plot_y0 = []
plot_x1 = []
plot_y1 = []

with open(INPUT) as csv_file:
    for row in csv.DictReader(csv_file):
        if int(row['priority']) == 0:
            plot_x0.append(float(row['time_ns'])    / 1000000)
            plot_y0.append(float(row['latency_ns']) / 1000000)
        elif int(row['priority']) == 1:
            plot_x1.append(float(row['time_ns'])    / 1000000)
            plot_y1.append(float(row['latency_ns']) / 1000000)

plt.scatter(plot_x0, plot_y0, s=10, label='high priority')
plt.scatter(plot_x1, plot_y1, s=10, label='low priority')
plt.legend()
plt.xlabel('Send time (ms)')
plt.ylabel('Latency (ms)')
plt.savefig(OUTPUT)
