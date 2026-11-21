# Chapter 5 — Worker Scaling Experiment (M6)

## Setup
- Cluster: k3d v5.9 (k8s v1.36), single k3d node (10 CPUs available to the VM), Rancher Desktop on macOS.
- Autoscaling: KEDA RabbitMQ scaler on queue `jobs`; for the controlled runs below the ScaledObject was pinned to a fixed replica count (`minReplicaCount = maxReplicaCount = N`).
- Workload: 120 identical `resize width=2000` jobs on a 4000×3000 (12 MP) JPEG, enqueued in parallel (16-way) so job submission is not the bottleneck.
- Measurement: queue-drain time — the clock starts once all 120 jobs are enqueued and stops when the 120th job reaches `COMPLETED` (polled from Postgres every 0.2 s). This isolates worker processing from submission latency.
- Each run starts from a truncated `jobs`/`outbox_events` table.
- Date: 2026-11-21.

## Results

| Workers | Jobs | Time (s) | Throughput (jobs/s) | Speedup | Efficiency |
|--------:|-----:|---------:|--------------------:|--------:|-----------:|
| 1 | 120 | 12.70 | 9.45 | 1.00 | 1.00 |
| 2 | 120 | 6.86 | 17.50 | 1.85 | 0.93 |
| 4 | 120 | 4.62 | 25.98 | 2.75 | 0.69 |

(Efficiency = speedup ÷ worker count: 1.0 is perfect linear scaling.)

## Analysis
- **1 → 2 workers: 1.85× speedup (93 % efficiency)** — near-linear. Each worker takes one message at a time (`prefetch_count=1`), so two workers pull from the shared `jobs` queue roughly evenly and the batch finishes in close to half the time.
- **1 → 4 workers: 2.75× speedup (69 % efficiency)** — still a clear gain, but with visible diminishing returns. The marginal worker adds less because contention moves off the worker CPU and onto shared resources: a single Postgres instance serialising status updates, one in-memory CloudServer handling every download/upload, and per-job fixed overhead (message round-trip, image decode/encode) that no amount of worker parallelism removes.
- **Why `prefetch_count=1` matters:** each worker holds exactly one unacked message, so the broker hands the next message to whichever worker is free. No single replica hoards the backlog while others idle — this even distribution is what turns added replicas into throughput. With a larger prefetch, a fast worker could grab a burst of messages and leave newcomers starved, flattening the curve.
- **Autoscaling behavior (unpinned, observed separately):** with the ScaledObject live (`value: 5` msgs/replica, min 1 / max 8), enqueuing a burst of 120 jobs drives the queue well above threshold and KEDA scales the Deployment toward `maxReplicaCount`; once the queue drains the HPA returns the Deployment to 1 replica after the stabilization window.
- **Caveat:** `S3BACKEND=mem` (CloudServer) and ephemeral Postgres storage mean a restart of either pod mid-run wipes state and invalidates that run. Every measurement above is a fresh, clean run. The absolute throughput numbers are specific to this single-node laptop cluster; the *shape* of the curve (near-linear, then tapering) is the portable result.

## Conclusion
The platform scales horizontally: throughput rose from 9.45 to 25.98 jobs/s (2.75×) going from 1 to 4 workers, near-linear through 2 workers and tapering at 4 as shared infrastructure becomes the limit. This validates the queue-based, `prefetch_count=1` design — adding stateless worker replicas increases throughput without any change to the rest of the system, which is exactly the scalability property the thesis set out to demonstrate.
