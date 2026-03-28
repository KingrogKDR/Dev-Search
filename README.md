# A Distributed Web Crawler from Scratch (Single-Node Architecture in Go)

A high-performance, single-machine web crawler built from scratch in Go.
Designed to explore large portions of the web efficiently while maintaining control over concurrency, politeness, and storage.
Here, the term _distributed_ refers to multiple crawlers, all working together as in a distributed system.
This is largely developer-focused and optimises for developer content. I have also added a basic indexer for 
future expansion into a search-engine.

## Contents

- [Overview](#Overview)
- [Architecture](#Architecture)


## Overview

This project is a **custom-built web crawler** that:

* Fetches and processes web pages concurrently
* Extracts and normalizes URLs
* Avoids duplicate crawling using deduplication strategies
* Respects crawl constraints (depth, domain, rate limits)
* Stores crawl data efficiently for analysis

The system is intentionally built on a **single-node architecture** to explore performance limits, system design trade-offs, and concurrency patterns in Go.

## Architecture

### Philosophy

Instead of relying on distributed systems upfront, this crawler focuses on:

* Maximizing **throughput on a single machine**
* Leveraging **Go's concurrency model**
* Keeping the system **simple, observable, and debuggable**
* Trying to keep the crawler developer-focused. So, `html` as well as `md` files are extracted.

### System Design

Basic Flow:

```
                +-------------------+
                |   Seed URLs       |
                +--------+----------+
                         |
                         v
                +-------------------+
                |   URL Frontier    |  (Queue / Scheduler)
                +--------+----------+
                         |
                         v
                +-------------------+
                |   Fetch Workers   |  (Concurrent Goroutines)
                +--------+----------+
                         |
                         v
                +-------------------+
                |   Parser          |
                +--------+----------+
                         |
                         v
                +-------------------+
                | URL Deduplicator  |
                +--------+----------+
                         |
                         v
                +-------------------+
                | Storage Layer     |
                +-------------------+
```

Crawler design:

![crawler system design](screenshots/crawler-design.png)



## 🔧 Core Components

### 1. URL Frontier (Scheduler)

* Maintains queue of URLs to crawl
* Supports:

  * FIFO / Priority-based scheduling
  * Depth control
  * Domain restrictions

---

### 2. Fetcher

* Responsible for HTTP requests
* Features:

  * Timeout handling
  * Retry logic
  * Rate limiting (per domain / global)

---

### 3. Worker Pool

* Goroutine-based concurrency model
* Configurable worker count
* Uses channels for communication

---

### 4. Parser

* Extracts:

  * Links (`<a href="">`)
  * Metadata (title, headers)
* Normalizes URLs:

  * Resolves relative paths
  * Removes fragments/query noise (optional)

---

### 5. Deduplication Engine

* Prevents re-crawling same URLs
* Techniques:

  * In-memory hash set / Bloom filter
  * URL normalization before hashing

---

### 6. Storage Layer

* Stores:

  * Crawled pages
  * Metadata
  * Link graph (optional)
* Options:

  * File-based (JSON / logs)
  * Embedded DB (BoltDB / SQLite)

---

## ⚙️ Configuration

Example:

```yaml
max_workers: 50
max_depth: 3
request_timeout: 5s
rate_limit_per_domain: 2
max_urls: 100000
```

---

## 📊 Performance & Stats

| Metric                 | Value  |
| ---------------------- | ------ |
| Max concurrent workers | 50     |
| Pages crawled/min      | ~X,XXX |
| Avg response time      | XXX ms |
| Duplicate rate         | XX%    |
| Memory usage           | XXX MB |

> Replace with real benchmarks.

---

## 🔍 Key Design Decisions

### Why Single Machine?

* Easier debugging
* Lower operational complexity
* Forces efficient resource usage

---

### Why Go?

* Lightweight concurrency (goroutines)
* Fast I/O handling
* Simple deployment (single binary)

---

### Why Channel-Based Communication?

* Avoid shared memory complexity
* Clear data flow between components

---

### Deduplication Strategy

* Trade-off between:

  * Memory usage
  * False positives (if Bloom filter used)

---

## ⚠️ Challenges Faced

* Handling duplicate URLs at scale
* Avoiding crawler traps (infinite URL spaces)
* Managing backpressure in worker pipelines
* Balancing concurrency vs rate limits

---

## 🧪 Future Improvements

* Distributed crawling (multi-node)
* Persistent URL frontier (disk-backed queue)
* Advanced politeness (robots.txt parsing)
* Content indexing (search engine integration)
* Adaptive crawling (priority based on content)

---

## ▶️ Getting Started

```bash
git clone https://github.com/yourusername/crawler
cd crawler
go run main.go
```

---

## 📌 Learnings

* Concurrency is easy to write, hard to control
* Bottlenecks shift from CPU → network → memory
* Simple systems scale surprisingly far when designed well

---

## 📄 License

MIT
