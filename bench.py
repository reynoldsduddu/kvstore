#!/usr/bin/env python3
import requests
import random
import string
import time
import argparse
import threading
from statistics import mean, median, quantiles
import subprocess
import datetime
recovery_lock = threading.Lock()
recovery_start = None
recovery_logged = False
last_leader = None
recovery_start = None
recovery_end = None


def kill_leader_delayed(leader_container, delay=5):
    def _delayed_kill():
        print(f"🕒 Waiting {delay}s before killing leader...")
        time.sleep(delay)
        print(f"💥 Stopping leader container: {leader_container}")
        subprocess.call(["docker", "stop", leader_container])
    threading.Thread(target=_delayed_kill, daemon=True).start()

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
    global recovery_start, recovery_logged
    initial_leader = get_leader(base_urls)
    local_latencies = []
    successes = 0

    for _ in range(num_ops):
        key = f"{thread_id}_{random_key()}"
        val = random_value()

        if target_mode == "leader":
            attempts = 0
            leader = None
            while attempts < 10:
                current_leader = get_leader(base_urls)
                #print(f"[Thread-{thread_id}] Attempt {attempts+1} → Leader: {current_leader}")
                if current_leader:
                    # Detect change in leader (true recovery)
                    with recovery_lock:
                        if initial_leader and current_leader != initial_leader and not recovery_logged:
                            recovery_end = datetime.datetime.now()
                            duration = (recovery_end - recovery_start).total_seconds()
                            print(f"✅ Leader changed from {initial_leader} to {current_leader} at {recovery_end.time()} — recovery time: {duration:.2f}s")
                            recovery_logged = True

                    # Set lost time if not already
                    with recovery_lock:
                        if not recovery_start:
                            recovery_start = datetime.datetime.now()
                            print(f"⚠️ Leader lost at {recovery_start.time()}")

                    leader = current_leader
                    break
                else:
                    time.sleep(1)
                    attempts += 1

            if not leader:
                print(f"[Thread-{thread_id}] Leader not found. Skipping.")
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
    # Auto-kill the leader after 5 seconds
    #kill_leader_delayed("node0", delay=5)
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
