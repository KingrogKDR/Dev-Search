# A Developer-Focused Web Crawler from Scratch (Single-Node Architecture in Go)

A developer focused crawler mainly for documentation and github search.

Docs and some github repos are a goldmine of learning and information. Many developers stay unaware of them or find them hard to discover.  Craw solves this problem.

People are free to contribute if they are willing. 

I have also added a basic indexer for future expansion into a search-engine.

## Table of Contents

* [Overview](#overview)
* [Architecture](#architecture)
* [Core Components](#core-components)
* [Performance & Stats](#performance--stats)
* [Key Design Decisions](#key-design-decisions)
* [Challenges Faced](#challenges-faced)
* [Getting Started](#getting-started)
* [Learnings](#learnings)


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
                |   URL Frontier    | 
                +--------+----------+
                         |
                         v
                +-------------------+
                |  Crawler Workers  |  
                +--------+----------+
                         |
                         v
                +-------------------+
                |  Parser Workers  |
                +--------+----------+
                         |
                         v
                +-------------------+
                | Storage Layer     |
                +-------------------+
```

Complete design:

![crawler system design](screenshots/crawler-design.png)


## Core Components

### 1. URL Frontier (Scheduler)

* Maintains queue of URLs to crawl
* Supports:

  * FIFO / Priority-based scheduling
  * Depth control
  * Domain restrictions
  * Retry Logic

It performs like a custom SQS with a visibility timer for retries. The flow can be represented as follows:

![Frontier Queue](screenshots/frontierSQS.png)


---

### 2. Fetcher

* Responsible for HTTP requests
* Features:

  * Timeout handling
  * Retry logic
  * robots.txt handling
  * Rate limiting per domain

---

### 3. Worker Pool

* Goroutine-based concurrency model
* Configurable worker count
* Uses redis queues for communication

---

### 4. Parser

* Extracts based on type:
  - html :
      * Links (`<a href="">`)
      * Metadata (title, headers)
      * Text Data
  - md:
      * Links
      * Headings
      * Description
      
* Deduplicates URLs

* Normalizes URLs:
  * Resolves relative paths
  * Removes fragments/query noise 

---

### 5. Deduplication Methods

* Prevents re-crawling same URLs (url-seen problem)
* Prevents duplicating content (content-seen problem)
* Techniques:

  * Simhash (for content dedup)
  * URL normalization before hashing and comparing the hash with existing (for url dedup)

---

### 6. Storage Layer

* Stores:

  * Crawled pages / Raw data `html/md`
  * Page Metadata 
  * Structured Documents for indexing
* Options:

  * File-based (JSON)
  * S3 (Minio)
  * Redis (for queues and for domain and url metadata)

---

## Performance & Stats

Crawler metrics: 

| Metric                 | Value  |
| ---------------------- | ------ |
| No. of crawlers        | 8      |
| No. of parsers         | 6      |
| Time elapsed           | 3hr 12min 21s|
| Pages crawled/sec      | 3.16   |
| Duplicate rate         | 30%    | 
| Avg fetch latency      | 1278.27 ms |
| Memory downloaded      | ~9.8 GB |

![Crawler metrics](screenshots/crawlerStats.png)

---


## 🔍 Key Design Decisions


### Why Single Machine?

I wanted to do this on a single machine for easier debugging, efficient resource usage and lower operational complexity. It helped me care more about learning and implementing the internals of a crawler which was my main goal, instead of focusing too much on distributed complexity.

### Why Go?

I mainly preferred Go for its easy-to-use concurrency features which can be optimised and hand-crafted as needed. It also has fast I/O handling and minimal syntax complexity to get started easily. It also has a simple deployment system (a single binary) and fast compile times.

### Why Redis Queues?

I first tried using channels but handling concurrency along with performance proved to be a serious bottleneck. Redis solved this with reliable atomic operations and queue primitives. Since it is highly configurable, it allowed me to keep my system design intact while smoothening concurrency burden. 

### Deduplication Strategy

I initially thought of using hashing for deduplication. It worked for urls after they were normalised, but not for content. This is because a slight change in content can result in a completely different hash. Since content in websites are impacted by ads, cookies and other forms of dynamic content, this can lead to a lot of false negatives.  

Therefore, I used simhashing for content-deduplication. It implemented near-duplication detection using Hamming Distance against a threshold. I have referenced a Google research paper on Simhash for implementing it.

Reference: https://lnkd.in/gGr-3uF4]](https://static.googleusercontent.com/media/research.google.com/en//pubs/archive/33026.pdf)

## Challenges Faced

* Handling duplicate for content
* Avoiding crawler traps (infinite URL spaces)
* Managing backpressure in worker pipelines
* Balancing concurrency vs rate limits
* Handling robots.txt and maintaining politeness.
* Error handling and preventing error inflation


## Getting Started

```bash
git clone https://github.com/KingrogKDR/Craw
docker compose up -d # run the dockerfile
go run .cmd/scraper # for running the crawler
go run .cmd/indexer # for running the indexer
```

## Learnings

* Internals of a crawler
* Concurrency bottlenecks, race conditions and context handling
* Performance and reliability tradeoffs
* Simple systems scale surprisingly far when designed well

Will craft my learnings in blogs soon.
