# Chapter 5 — Kubernetes + Scaling (M5/M6)

**Date:** 2026-11-21
**Status:** Design — awaiting approval
**Goal:** Run the whole Image Processing Platform on a local Kubernetes cluster, autoscale the processing-worker on RabbitMQ queue depth, and produce a scaling experiment (1/2/4 workers → throughput) with written analysis for the thesis.

## Intent (brief back)

- **What:** Deploy all services to K8s; make the worker horizontally autoscale based on how many jobs are waiting in the queue; measure and document how throughput changes with replica count.
- **Why:** This is the core thesis result (M5 = runs on K8s, M6 = demonstrated scalability). The platform was deliberately built queue-based with `prefetch_count=1` precisely so adding workers increases throughput — Chapter 5 proves it.
- **Success:** `make k8s-up` brings up a working cluster with the frontend reachable via Ingress; the HPA scales the worker up under load and back down when the queue drains; a results doc shows throughput at 1/2/4 workers with commentary.

## Decisions (confirmed with user)

| Decision | Choice |
|---|---|
| Cluster tool | **k3d** (k3s-in-Docker, built-in local registry) |
| HPA metric | **RabbitMQ queue depth via KEDA** (`ScaledObject`, rabbitmq trigger on queue `jobs`) |
| Scaling experiment | **Load script + markdown results doc** (submit N jobs, measure throughput at 1/2/4 workers) |
| Stateful infra | **In-cluster, ephemeral** (emptyDir; no PVCs — resets on restart, fine for a throughput demo) |
| Ingress | **Traefik** (bundled with k3d) |

## Architecture

### Namespace
Everything in a single namespace `image-platform`.

### Workloads

| Component | Kind | Replicas | Notes |
|---|---|---|---|
| postgres | Deployment | 1 | emptyDir volume; migrations loaded via ConfigMap mounted at `/docker-entrypoint-initdb.d` |
| rabbitmq | Deployment | 1 | `3.13-management-alpine`; AMGP 5672 + mgmt 15672; emptyDir |
| minio (CloudServer) | Deployment | 1 | `zenko/cloudserver:latest-7.70.10`, `S3BACKEND=mem`, `ENDPOINT=minio`; port 8000 |
| api-service | Deployment | 1 | built image; `/healthz` readiness + liveness probes |
| job-service | Deployment | 1 | built image; `/healthz` probes; runs the outbox relay goroutine |
| processing-worker | Deployment | 1 (HPA 1→N) | built image; **no probes needed** (no HTTP server); the autoscale target |
| frontend | Deployment | 1 | nginx; keeps its existing internal routing to api-service/job-service |

### Services (ClusterIP)
`postgres:5432`, `rabbitmq:5672` + `rabbitmq:15672`, `minio:8000`, `api-service:8080`, `job-service:8081`, `frontend:80`. Service names match docker-compose hostnames so **no app env changes are required**.

### Ingress (Traefik)
Single host (e.g. `platform.localhost` or the k3d loadbalancer IP). Routing is kept **inside the frontend's existing nginx** rather than duplicated at the Ingress, so:
- Ingress: `/` → `frontend:80` (catch-all).
- The frontend nginx continues to proxy `/api/v1/images` → api-service, `/api/v1/jobs/{id}/result` → api-service (the ordering-sensitive exception, an existing invariant), `/api/v1/jobs` → job-service.

This preserves the documented `/result`-before-`/jobs` location ordering and keeps routing in one place. (The CLAUDE.md "ingress routes images→api, jobs→job" decision is satisfied functionally; we note in the thesis that the split happens at the frontend proxy tier.)

### Config & secrets
- **ConfigMap `platform-config`**: non-secret env (ports, `MINIO_ENDPOINT=minio:8000`, `MINIO_USE_SSL=false`, `MAX_UPLOAD_SIZE_MB`, `MAX_MEGAPIXELS`, `PRESIGNED_URL_TTL_MINUTES`, `OUTBOX_POLL_MS`, `WORKER_LOG_LEVEL`, `S3BACKEND=mem`, etc.).
- **ConfigMap `db-migrations`**: the two SQL files, mounted into the postgres initdb dir.
- **Secret `platform-secrets`** (from `secrets.example/` template, gitignored real values): `POSTGRES_PASSWORD`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `RABBITMQ_DEFAULT_PASS`, and the composed `DATABASE_URL` / `RABBITMQ_URL`. `DATABASE_URL` and `RABBITMQ_URL` contain credentials so they live in the Secret, not the ConfigMap. **Note:** `MINIO_ACCESS_KEY`/`MINIO_SECRET_KEY` are consumed only by the *clients* (api-service, worker); the CloudServer pod uses its built-in default account and receives no credential env (see Startup-ordering hazard #3).

### Autoscaling (KEDA)
- Install KEDA into the cluster (`make k8s-up` runs `kubectl apply` of the KEDA release manifest).
- **`ScaledObject`** targeting the `processing-worker` Deployment:
  - trigger: `rabbitmq`, `queueName: jobs`, `mode: QueueLength`, `value: "5"` (scale so ~5 queued msgs per replica), `host` from a `TriggerAuthentication` referencing the AMQP URL secret.
  - `minReplicaCount: 1`, `maxReplicaCount: 8`, cooldown tuned short (e.g. 30s) so experiments don't wait minutes for scale-down.
- KEDA creates the underlying HPA automatically; the thesis can show either the `ScaledObject` or the generated HPA.

> Note: queue-depth scaling is KEDA's job. metrics-server is still installed for `kubectl top` so the thesis can also report CPU/memory per worker.

## Directory layout (fills the existing scaffold)

```
infrastructure/kubernetes/
  namespace.yaml
  configmaps/        platform-config.yaml, db-migrations.yaml
  secrets.example/   platform-secrets.example.yaml   (real secrets.yaml gitignored)
  storage/           (emptyDir inline — kept minimal; dir may stay a placeholder)
  deployments/       postgres, rabbitmq, minio, api-service, job-service, processing-worker, frontend
  services/          one Service per deployment + ingress.yaml
  autoscaling/       keda-scaledobject.yaml, trigger-auth.yaml
```

## Build & image flow
k3d has an integrated registry. `make k8s-build` builds the four service images (`docker build`) and imports them into the cluster (`k3d image import` into the k3d cluster), tagged `image-platform/<svc>:dev`. Deployments reference those tags with `imagePullPolicy: IfNotPresent`.

## Makefile targets (new)
```
make k8s-up        # create k3d cluster, install KEDA + metrics-server, build+import images, apply manifests
make k8s-down      # delete the k3d cluster
make k8s-status    # kubectl get pods,svc,scaledobject,hpa -n image-platform
make k8s-logs      # tail worker logs
make scale-test    # run the scaling experiment script
```

## Scaling experiment
- **`scripts/scale-test.sh`**: for each worker count in {1, 2, 4}:
  1. pin replicas (temporarily set `minReplicaCount=maxReplicaCount=N` or scale deployment and pause KEDA) so the run is controlled,
  2. submit a fixed batch of jobs (e.g. 60) via the Ingress using the same upload+create-job flow as the smoke test,
  3. poll until all jobs reach `COMPLETED`,
  4. record wall-clock time → compute throughput (jobs/sec) and speedup vs 1 worker.
- A second mode runs with **KEDA live** (no pin) to demonstrate automatic scale-up/scale-down, capturing replica count over time.
- **`docs/results/chapter-5-scaling.md`**: table of (replicas, total time, throughput, speedup, efficiency) + short analysis (near-linear up to CPU saturation / queue-bound behavior, where it plateaus, and why `prefetch_count=1` matters).

## Error handling & correctness
- Readiness probes on api-service/job-service gate traffic until DB/queue deps are reachable (mirrors compose `depends_on: service_healthy`).
- Worker has no HTTP server → use a simple liveness check (process up) or none; rely on `restartPolicy: Always`.
- Ephemeral storage means a postgres restart wipes data; acceptable because each experiment run starts from a known batch. Documented explicitly so results are reproducible.
- KEDA `TriggerAuthentication` pulls the AMQP URL from the Secret — the queue host/credential is never inlined in the `ScaledObject`.

### Startup-ordering hazards (verified against current code)
These three couplings are hidden by docker-compose `depends_on` and must be handled explicitly in K8s:

1. **nginx resolves upstreams at startup.** `frontend/nginx.conf` uses literal `proxy_pass http://api-service:8080` / `http://job-service:8081` with **no `resolver` directive**. Stock nginx resolves these hostnames **once at boot**; if the frontend pod starts before the `api-service`/`job-service` Services have endpoints, nginx either fails to start or caches an NXDOMAIN and never recovers. **Fix:** add an init-container on the frontend that waits for both Services (or add a `resolver <kube-dns-ip> valid=10s;` + variabilized upstreams). Init-container wait is simpler and matches the compose semantics — chosen.

2. **Only api-service creates buckets.** `api-service` calls `EnsureBuckets` on boot (creates `images-original` + `images-processed`). The **worker does not** — `storage.upload_result` calls `put_object` on `images-processed` assuming it exists. **Fix:** worker Deployment waits (init-container) until api-service is Ready before consuming, so buckets exist first. Order: minio → api-service (ensures buckets) → worker.

3. **CloudServer credentials are image defaults, NOT configured by us.** The `minio` service in compose receives **no** access/secret-key env — `accessKey1`/`verySecretKey1` are CloudServer's built-in default account baked into the image. The `MINIO_ACCESS_KEY`/`MINIO_SECRET_KEY` values in the Secret configure the **clients** (api-service, worker) only; the CloudServer Deployment gets just `S3BACKEND=mem`, `REMOTE_MANAGEMENT_DISABLE=1`, `ENDPOINT=minio`. Do not inject client keys into the server pod expecting them to take effect.

> Consequence of ephemeral `S3BACKEND=mem` + finding #2: if the CloudServer pod restarts mid-run, all buckets vanish and only an api-service restart re-creates them (the worker can't). Fine for a clean experiment run (which is how `scale-test.sh` operates — fresh batch each time), but the results doc must state that a CloudServer restart invalidates an in-flight run.

## Testing gate (per CLAUDE.md)
- `make test-unit`, `make build`, `make lint` unaffected (no app code changes expected).
- New: a **K8s smoke test** — after `make k8s-up`, run the existing end-to-end flow (upload → create job → poll COMPLETED → download result) through the Ingress, asserting HTTP 200 + non-empty result body. This also closes the existing smoke-test gap (result download not currently asserted).
- `make lint` extension: optionally `kubeconform`/`kubectl --dry-run=client` validate manifests if available; otherwise skip gracefully.

## Out of scope
- Persistent storage / backups (ephemeral by decision).
- TLS/real DNS (localhost only).
- Multi-node cluster (single k3d node; worker scaling is pod-level, which is what the thesis measures).
- Production secret management (K8s Secret with a gitignored real file is sufficient for thesis scope).

## Open risk
- KEDA + k3d version compatibility and the exact RabbitMQ trigger auth format should be verified against current KEDA docs during implementation (flagged as the one thing to check live rather than from memory).
