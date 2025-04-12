# plot_cpu.py
import matplotlib.pyplot as plt
import csv

timestamps, usages = [], []
with open("cpu_log.txt", "r") as f:
    reader = csv.reader(f)
    next(reader)
    for row in reader:
        timestamps.append(float(row[0]))
        usages.append(float(row[1]))

plt.figure(figsize=(10, 5))
plt.plot(timestamps, usages, marker='o', linestyle='-', color='blue')
plt.title("CPU Usage During Benchmark")
plt.xlabel("Time (s)")
plt.ylabel("CPU Usage (%)")
plt.grid(True)
plt.savefig("cpu_usage_plot.png")
plt.show()
