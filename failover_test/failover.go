package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

type Result struct {
	successes int
	latencies []float64
}

func randomKey(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func sendPutRequest(url, key, value string) (bool, float64) {
	client := &http.Client{Timeout: 5 * time.Second}
	payload := map[string]string{"key": key, "value": value}
	data, _ := json.Marshal(payload)

	start := time.Now()
	resp, err := client.Post("http://"+url+"/api/put", "application/json", bytes.NewReader(data))
	if err != nil {
		//fmt.Printf("❌ Failed PUT to %s: %v\n", url, err)
		return false, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, time.Since(start).Seconds() * 1000
	}

	// 🔍 Print failure reason
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Printf("❌ PUT failed [%d] to %s: %s\n", resp.StatusCode, url, buf.String())

	return false, 0
}

func isAlive(url string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://" + url + "/api/heartbeat")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func getLeader(targets []string) string {
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 10; i++ {
		for _, target := range targets {
			if !isAlive(target) {
				continue
			}

			resp, err := client.Get("http://" + target + "/api/leader")
			if err != nil || resp.StatusCode != 200 {
				continue
			}

			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
				resp.Body.Close()
				leader := strings.TrimSpace(body["leader"])
				// Map internal name like node0:8081 → localhost:8081
				parts := strings.Split(leader, ":")
				if len(parts) == 2 {
					port := parts[1]
					mapped := "localhost:" + port
					return mapped
				}
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return ""
}

func killContainer(containerName string) {
	cmd := exec.Command("docker", "kill", containerName)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("❌ Failed to kill container '%s': %v\n", containerName, err)
	} else {
		fmt.Printf("💀 Killed leader container: %s\n", containerName)
	}
}

func killLeaderAfterDelay(delay time.Duration, targets []string, containerMap map[string]string) {
	go func() {
		fmt.Printf("[INFO] Will kill the leader after %.0f seconds...\n", delay.Seconds())

		// ✅ Fetch the leader NOW
		leader := getLeader(targets)
		leader = strings.TrimSpace(leader)
		fmt.Println("[LEADER] Detected leader before delay:", leader)

		time.Sleep(delay)

		if leader == "" {
			fmt.Println("⚠️ No leader was found before delay. Aborting kill.")
			return
		}

		container := containerMap[leader]
		fmt.Printf("[INFO] Leader %s → container %s\n", leader, container)

		if container == "" {
			fmt.Println("⚠️ No container mapped to leader:", leader)
			return
		}

		killContainer(container)

		// Optional: Detect new leader
		start := time.Now()
		timeout := 15 * time.Second
		var lastLeader string

		for {
			time.Sleep(1 * time.Second)
			newLeader := getLeader(targets)
			if newLeader != "" && newLeader != leader {
				if newLeader != lastLeader {
					fmt.Printf("[INFO] New leader detected: %s\n", newLeader)
					lastLeader = newLeader
				}
				elapsed := time.Since(start).Seconds()
				fmt.Printf("[NEW LEADER] %s elected after %.2f seconds\n", newLeader, elapsed)
				return
			}
			if time.Since(start) > timeout {
				fmt.Println("[TIMEOUT] No new leader detected.")
				return
			}
		}
	}()
}

func worker(threadID, ops int, mode string, targets []string, wg *sync.WaitGroup, results *[]Result, mu *sync.Mutex) {
	defer wg.Done()
	r := Result{}
	for i := 0; i < ops; i++ {
		key := fmt.Sprintf("%d_%s", threadID, randomKey(8))
		value := randomKey(16)
		var target string
		if mode == "cabinet" {
			target = getLeader(targets)
			if target == "" {
				fmt.Println("[WARN] No leader found, skipping.")
				time.Sleep(300 * time.Millisecond)
				continue
			}
		} else {
			target = targets[rand.Intn(len(targets))]
		}

		//fmt.Printf("[Thread %d] Sending PUT to %s\n", threadID, target)

		success, latency := sendPutRequest(target, key, value)
		if success {
			r.successes++
			r.latencies = append(r.latencies, latency)
		}
	}
	mu.Lock()
	*results = append(*results, r)
	mu.Unlock()
}

func main() {
	var mode string
	var concurrency int
	var ops int
	var targetsCSV string

	flag.StringVar(&mode, "mode", "cabinet", "Consensus mode: cabinet or cabinet++")
	flag.IntVar(&concurrency, "concurrency", 1, "Number of concurrent clients")
	flag.IntVar(&ops, "ops", 100, "Total number of PUT operations")
	flag.StringVar(&targetsCSV, "targets", "localhost:8081,localhost:8082,localhost:8083,localhost:8084,localhost:8085", "Comma-separated list of node addresses")

	flag.Parse()

	targets := strings.Split(targetsCSV, ",")

	containerMap := map[string]string{
		"node0:8081":     "node0",
		"node1:8081":     "node1",
		"node2:8081":     "node2",
		"node3:8081":     "node3",
		"node4:8081":     "node4",
		"localhost:8081": "node0",
		"localhost:8082": "node1",
		"localhost:8083": "node2",
		"localhost:8084": "node3",
		"localhost:8085": "node4",
	}

	var wg sync.WaitGroup
	var results []Result
	var mu sync.Mutex

	perThreadOps := ops / concurrency
	fmt.Printf("🚀 Starting benchmark: mode=%s, concurrency=%d, ops=%d\n", mode, concurrency, ops)
	start := time.Now()

	// ✅ Start workers BEFORE scheduling leader kill
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker(i, perThreadOps, mode, targets, &wg, &results, &mu)
	}

	// ✅ Schedule kill with slightly longer delay to allow pre-kill writes
	killLeaderAfterDelay(5*time.Second, targets, containerMap)

	wg.Wait()

	duration := time.Since(start).Seconds()
	totalSuccess := 0
	var allLatencies []float64
	for _, r := range results {
		totalSuccess += r.successes
		allLatencies = append(allLatencies, r.latencies...)
	}

	fmt.Println("\n📊 Benchmark Results")
	fmt.Printf("✅ Success: %d/%d\n", totalSuccess, ops)
	fmt.Printf("⏱️ Duration: %.2fs\n", duration)
	fmt.Printf("🚀 Throughput: %.2f ops/sec\n", float64(totalSuccess)/duration)

	if len(allLatencies) > 0 {
		sum := 0.0
		for _, l := range allLatencies {
			sum += l
		}
		avg := sum / float64(len(allLatencies))

		sort.Float64s(allLatencies)
		p95 := allLatencies[int(0.95*float64(len(allLatencies)))]
		p99 := allLatencies[int(0.99*float64(len(allLatencies)))]

		fmt.Printf("⏱️ Avg Latency: %.2f ms\n", avg)
		fmt.Printf("📈 P95 Latency: %.2f ms\n", p95)
		fmt.Printf("📈 P99 Latency: %.2f ms\n", p99)
	} else {
		fmt.Println("❌ No successful operations recorded.")
	}
}
