# cpu_monitor.py
import time, psutil, argparse

def log_cpu(interval=1.0, duration=60):
    with open("cpu_log.txt", "w") as f:
        f.write("Time(s),CPU_Usage(%)\n")
        for i in range(int(duration / interval)):
            usage = psutil.cpu_percent(interval=interval)
            f.write(f"{i*interval},{usage}\n")
            print(f"[CPU MONITOR] {i*interval:.1f}s: {usage}%")

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--interval", type=float, default=1.0)
    parser.add_argument("--duration", type=int, default=60)
    args = parser.parse_args()
    log_cpu(args.interval, args.duration)
