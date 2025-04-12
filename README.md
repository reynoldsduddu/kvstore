# Distributed Key-Value Store using Cabinet and Cabinet++

This project implements a distributed key-value store with dynamic quorum-based consensus using **Cabinet** and **Cabinet++**, based on the VLDB'24 paper *“Cabinet: Dynamically Weighted Consensus Made Fast”* by Zhang et al.

The system is built in **Go**, supports fault-tolerant writes, automatic failover, dynamic weight reassignment, and a full-featured **JavaScript frontend** for real-time consensus visualization and interaction.

---

## 🔧 Features

- ⚖️ Dynamic quorum consensus using Cabinet and Cabinet++
- 🔄 Automatic leader election and heartbeat-based liveness
- 📊 Real-time Cabinet weight visualization with Chart.js
- 🧪 Benchmarking tools for latency, throughput, and failover tests
- 🌐 RESTful API with support for PUT, GET, DELETE, and GET-ALL
- 🐳 Dockerized 5-node deployment with SQLite-backed persistence

---

## 🚀 Getting Started

### 1. Clone the Repository

```bash
git clone https://github.com/reynoldsduddu/kvstore.git
cd kvstore
```

### 2. Build and Run with Docker

Ensure Docker and Docker Compose are installed.

```bash
docker-compose up --build
```

This starts five nodes on:
```
http://localhost:8081
http://localhost:8082
http://localhost:8083
http://localhost:8084
http://localhost:8085
```

By default, the system runs in `cabinet` mode. See below to enable Cabinet++.

---

## 🖥️ Frontend

Open your browser and navigate to:

```
http://localhost:8081
```

Features:
- Bar chart of Cabinet weights and node health
- View current leader and consensus mode
- Submit key-value operations (PUT, GET, DELETE)
- View all stored key-value pairs with pagination

---

## ⚙️ Switch Between Cabinet and Cabinet++

Edit the `docker-compose.yml` to set:

```yaml
- CONSENSUS_MODE=cabinet++
```

Then rebuild:

```bash
docker-compose down
docker-compose up --build
```

---

## 📈 Benchmarking

Two benchmarking tools are provided:

### `bench.go` — Normal Benchmark

```bash
go run bench.go --mode cabinet++ --concurrency 5 --ops 500
```

### `failover.go` — Leader Failover Test

```bash
go run failover.go --mode cabinet --concurrency 10 --ops 500
```

Both scripts report success rate, throughput, and latency statistics (Avg, P95, P99).

---

## 📂 Folder Structure

| Folder/File         | Description                              |
|---------------------|------------------------------------------|
| `main.go`           | Cluster bootstrap logic                  |
| `consensus/`        | Cabinet/Cabinet++ consensus implementation |
| `kvstore/`          | Key-value operations and HTTP handlers   |
| `frontend/`         | `index.html`, `script.js`, `style.css`  |
| `bench.go`          | Benchmark script                         |
| `failover.go`       | Benchmark with leader termination        |
| `docker-compose.yml`| Container setup                          |

---

## 📜 License

This project is released under the MIT License.

---

## 🙏 Acknowledgments

This implementation is based on the Cabinet consensus protocol introduced by Zhang et al. (VLDB 2024).

> Zhang, G., Pan, S., Gupta, A., & Stutsman, R. (2024). *Cabinet: Dynamically Weighted Consensus Made Fast*. Proceedings of the VLDB Endowment, 17(1), 28–40.