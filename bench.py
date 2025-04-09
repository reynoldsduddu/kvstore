#!/usr/bin/env python3
import requests
import random
import string
import time
import argparse
import threading
from statistics import mean, median, quantiles

def random_key(length=8):
    return ''.join(random.choices(string.ascii_letters + string.digits, k=length))

def random_value(length=16):
    return ''.join(random.choices(string.ascii_letters + string.digits, k=length))

def get_leader(base_urls):
    for url in base_urls:
        try:
            res = requests.get(f"http://{url}/api/leader", timeout=1)
            if res.status_code == 200:
                leader = res.json()["leader"]
                # Map internal name (e.g., node0:8081) to exposed port (e.g., localhost:8081)
                for u in base_urls:
                    if leader.endswith(u.split(":")[1]):  # match port
                        return u
        except:
            continue
    return None


def send_put(url, key, value):
    try:
        start = time.time()
        res = requests.put(f"http://{url}/api/put", json={"key": key, "value": value}, timeout=5)
        latency = (time.time() - start) * 1000  # ms
        return res.status_code == 200, latency
    except Exception as e:
        return False, None

def worker(thread_id, num_ops, target_mode, base_urls, result_list):
    local_latencies = []
    successes = 0

    for _ in range(num_ops):
        key = f"{thread_id}_{random_key()}"
        val = random_value()

        if target_mode == "leader":
            leader = get_leader(base_urls)
            if not leader:
                print("[WARN] Leader not found. Skipping.")
                continue
            target = leader
        else:
            target = random.choice(base_urls)

        ok, latency = send_put(target, key, val)
        if ok:
            successes += 1
            local_latencies.append(latency)
    
    result_list.append((successes, local_latencies))

def main():
    parser = argparse.ArgumentParser(description="Cabinet/Cabinet++ Benchmarking Tool")
    parser.add_argument("--mode", choices=["cabinet", "cabinet++"], required=True, help="Test mode")
    parser.add_argument("--concurrency", type=int, default=1, help="Number of concurrent clients")
    parser.add_argument("--ops", type=int, default=100, help="Total number of PUT operations")
    parser.add_argument("--targets", nargs="+", default=["localhost:8081", "localhost:8082", "localhost:8083", "localhost:8084", "localhost:8085"],
                        help="List of node addresses (host:port)")
    args = parser.parse_args()

    per_thread_ops = args.ops // args.concurrency
    threads = []
    results = []

    print(f"Starting benchmark: mode={args.mode}, concurrency={args.concurrency}, total_ops={args.ops}")
    start_time = time.time()

    for i in range(args.concurrency):
        t = threading.Thread(target=worker, args=(i, per_thread_ops, "leader" if args.mode == "cabinet" else "random", args.targets, results))
        threads.append(t)
        t.start()

    for t in threads:
        t.join()

    total_success = sum(r[0] for r in results)
    all_latencies = [lat for r in results for lat in r[1]]
    duration = time.time() - start_time

    if all_latencies:
        avg_latency = mean(all_latencies)
        p95_latency = quantiles(all_latencies, n=100)[94]
        p99_latency = quantiles(all_latencies, n=100)[98]
    else:
        avg_latency = p95_latency = p99_latency = 0

    print("Benchmark Results")
    print(f"Success: {total_success}/{args.ops}")
    print(f"Duration: {duration:.2f}s")
    print(f"Throughput: {total_success/duration:.2f} ops/sec")
    print(f"Avg Latency: {avg_latency:.2f} ms")
    print(f"P95 Latency: {p95_latency:.2f} ms")
    print(f"P99 Latency: {p99_latency:.2f} ms")

if __name__ == "__main__":
    main()
