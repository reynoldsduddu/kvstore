#!/usr/bin/env python3
import requests, random, string, time, argparse, threading, os
from statistics import mean, quantiles

leader_url = None  # Used for Cabinet mode (leader-based)

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
                for u in base_urls:
                    if leader.endswith(u.split(":")[1]):
                        return u
        except:
            continue
    return None

def wait_for_leader_change(old_leader, targets, timeout=30):
    print(f"⏳ Waiting for leader to change from {old_leader}...")
    start = time.time()
    while time.time() - start < timeout:
        new_leader = get_leader(targets)
        if new_leader and new_leader != old_leader:
            print(f"👑 New leader is {new_leader}")
            return new_leader, time.time() - start
        time.sleep(0.2)
    print("❌ Leader did not change within timeout.")
    return None, None

def send_put(url, key, value):
    try:
        start = time.time()
        res = requests.put(f"http://{url}/api/put", json={"key": key, "value": value}, timeout=5)
        latency = (time.time() - start) * 1000
        return res.status_code == 200, latency
    except:
        return False, None

def log_alive_nodes(base_urls):
    print("🔍 Checking node liveness...")
    for url in base_urls:
        try:
            res = requests.get(f"http://{url}/api/status", timeout=2)
            if res.status_code == 200:
                status = res.json()
                for node, alive in status.items():
                    state = "🟢 Alive" if alive else "🔴 Dead"
                    print(f"  {node}: {state}")
                return
        except:
            continue
    print("⚠️ Could not fetch node status from any peer.")

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
def get_container_name_for_leader(leader_url):
    port = leader_url.split(":")[1]
    return {
        "8081": "node0",
        "8082": "node1",
        "8083": "node2",
        "8084": "node3",
        "8085": "node4",
    }.get(port, None)

def main():
    global leader_url
    parser = argparse.ArgumentParser(description="Cabinet/Cabinet++ Benchmark with failover and verification")
    parser.add_argument("--mode", choices=["cabinet", "cabinet++"], required=True)
    parser.add_argument("--concurrency", type=int, default=1)
    parser.add_argument("--ops", type=int, default=100)
    parser.add_argument("--targets", nargs="+", default=[
        "localhost:8081", "localhost:8082", "localhost:8083", "localhost:8084", "localhost:8085"
    ])
    parser.add_argument("--kill-leader-after", type=int, help="Seconds to wait before killing the leader")
    args = parser.parse_args()

    log_alive_nodes(args.targets)

    print(f"🚀 Starting benchmark: mode={args.mode}, concurrency={args.concurrency}, total_ops={args.ops}")
    per_thread_ops = args.ops // args.concurrency
    threads = []
    results = []

    leader = get_leader(args.targets)
    if not leader:
        print("❌ No leader found. Aborting benchmark.")
        return
    leader_url = leader

    if args.kill_leader_after:
        print(f"💣 Will kill leader {leader} after {args.kill_leader_after} seconds")
        time.sleep(args.kill_leader_after)

        container_name = get_container_name_for_leader(leader)
        if not container_name:
            print(f"❌ Could not map {leader} to a container. Aborting.")
            return

        print(f"💀 Killing leader container: {container_name}")
        os.system(f"docker kill {container_name}")

        new_leader, elapsed = wait_for_leader_change(leader, args.targets)
        if elapsed:
            print(f"✅ Re-election completed in {elapsed:.2f} seconds")
            if args.mode == "cabinet":
                leader_url = new_leader
                print(f"🔁 Redirecting future PUTs to new leader: {leader_url}")
        else:
            print("❌ Re-election failed. Skipping PUTs.")
            return


    start_time = time.time()
    for i in range(args.concurrency):
        t = threading.Thread(
            target=worker,
            args=(i, per_thread_ops, "leader" if args.mode == "cabinet" else "random", args.targets, results)
        )
        threads.append(t)
        t.start()

    for t in threads:
        t.join()

    duration = time.time() - start_time
    total_success = sum(r[0] for r in results)
    latencies = [l for r in results for l in r[1]]

    if latencies:
        avg = mean(latencies)
        p95 = quantiles(latencies, n=100)[94]
        p99 = quantiles(latencies, n=100)[98]
    else:
        avg = p95 = p99 = 0

    print("📊 Benchmark Results")
    print(f"✅ Success: {total_success}/{args.ops}")
    print(f"⏱️ Duration: {duration:.2f}s")
    print(f"🚀 Throughput: {total_success/duration:.2f} ops/sec")
    print(f"⏱️ Avg Latency: {avg:.2f} ms")
    print(f"📈 P95 Latency: {p95:.2f} ms")
    print(f"📈 P99 Latency: {p99:.2f} ms")

if __name__ == "__main__":
    main()
