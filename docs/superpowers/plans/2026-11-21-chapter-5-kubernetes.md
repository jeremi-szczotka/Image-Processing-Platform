# Chapter 5 — Kubernetes + Scaling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy all 7 platform services to a local k3d Kubernetes cluster, autoscale the processing-worker on RabbitMQ queue depth via KEDA, and produce a 1/2/4-replica scaling experiment with written throughput analysis.

**Architecture:** Single `image-platform` namespace. Stateful infra (postgres, rabbitmq, CloudServer) and app services (api-service, job-service, processing-worker, frontend) run as Deployments with ClusterIP Services whose names match the docker-compose hostnames, so no app env changes are required. Traefik Ingress routes `/` → frontend, and the frontend's existing nginx proxies to api-service/job-service. KEDA's `ScaledObject` scales the worker from the RabbitMQ `jobs` queue length. All manifests are plain YAML applied with `kubectl apply`; the cluster lifecycle and image import are driven from the Makefile.

**Tech Stack:** k3d v5.9 (k8s v1.36), Traefik (bundled), KEDA (rabbitmq scaler), metrics-server, kubectl, bash.

**Spec:** `docs/superpowers/specs/2026-11-21-chapter-5-kubernetes-design.md`

## Global Constraints

- **Namespace:** everything in `image-platform` (except KEDA, which installs into `keda`, and metrics-server into `kube-system`).
- **Service names must equal compose hostnames** — `postgres`, `rabbitmq`, `minio`, `api-service`, `job-service`, `frontend`. App env (`DATABASE_URL`, `RABBITMQ_URL`, `MINIO_ENDPOINT=minio:8000`) is reused verbatim; do NOT change app code or env var names.
- **Image tags:** `image-platform/<svc>:dev` for all four built services; Deployments use `imagePullPolicy: IfNotPresent`.
- **Cluster name:** `image-platform` (k3d). kubectl context after create: `k3d-image-platform`.
- **Queue name is `jobs`** (durable) — declared by both job-service and worker; KEDA trigger must use `queueName: jobs`.
- **CloudServer image:** `zenko/cloudserver:latest-7.70.10`, env `S3BACKEND=mem`, `REMOTE_MANAGEMENT_DISABLE=1`, `ENDPOINT=minio`. It receives NO access/secret-key env (uses built-in `accessKey1`/`verySecretKey1`).
- **S3 client keys** (`accessKey1`/`verySecretKey1`) and DB/RabbitMQ creds live in the `platform-secrets` Secret; non-secret config in `platform-config` ConfigMap.
- **Secrets never committed:** only `secrets.example/platform-secrets.example.yaml` with placeholder values is tracked; the real `secrets.yaml` is gitignored.
- **GoDoc / comment conventions** from CLAUDE.md apply only to Go code — no Go code is added here, so N/A.
- **Testing gate:** `make test-unit`, `make build`, `make lint` must stay green (no app code changes). New `make k8s-smoke` must pass against a running cluster before the chapter is committed as done.

## Review Focus

Inputs/conditions the spec implies but that no single task's happy-path test exercises — most likely to bite first:

1. **Frontend pod starts before api-service/job-service Services have endpoints** → nginx resolves `proxy_pass` hostnames once at boot, caches NXDOMAIN, never recovers. Pinned by the init-container wait in Task 9 and asserted in the k8s-smoke test (Task 12) via a fresh `make k8s-up` where pod start order is not guaranteed.
2. **Worker consumes a job before api-service has run `EnsureBuckets`** → `put_object` on `images-processed` fails, job → FAILED. Pinned by the worker init-container wait-for-api-service-Ready in Task 8.
3. **KEDA secret holds the AMQP URL unencoded / wrong key name** → `TriggerAuthentication` can't authenticate, worker never scales. Pinned by Task 10 using `stringData` (not `data`) so no manual base64, and `parameter: host` matching the scaler's expected field.
4. **CloudServer pod restart mid-experiment wipes buckets** → subsequent uploads 500. Documented as a run-invalidating condition in the results doc (Task 13); scale-test.sh starts each run fresh (Task 11).
5. **Ingress host not resolvable from the host machine** → smoke/load scripts can't reach the cluster. Pinned by Task 7 using the k3d loadbalancer on `localhost` with an explicit port mapping, and the scripts targeting `http://localhost:<port>` with a `Host:` header rather than relying on DNS.

---

## File Structure

```
infrastructure/kubernetes/
  namespace.yaml                              # Task 1
  configmaps/platform-config.yaml             # Task 2
  configmaps/db-migrations.yaml               # Task 2 (generated from database/migrations/)
  secrets.example/platform-secrets.example.yaml  # Task 3
  storage/.gitkeep                            # (unchanged — emptyDir is inline)
  deployments/postgres.yaml                   # Task 4
  deployments/rabbitmq.yaml                   # Task 5
  deployments/minio.yaml                      # Task 6
  deployments/api-service.yaml                # Task 7
  deployments/job-service.yaml                # Task 7
  deployments/frontend.yaml                   # Task 9
  deployments/processing-worker.yaml          # Task 8
  services/postgres.yaml rabbitmq.yaml minio.yaml api-service.yaml job-service.yaml frontend.yaml  # Tasks 4-9
  services/ingress.yaml                       # Task 7
  autoscaling/trigger-auth.yaml               # Task 10
  autoscaling/keda-scaledobject.yaml          # Task 10
.gitignore                                    # Task 3 (add secrets.yaml)
Makefile                                      # Task 11 (k8s-* targets)
scripts/k8s-smoke.sh                          # Task 12
scripts/scale-test.sh                         # Task 11/13
docs/results/chapter-5-scaling.md             # Task 13
```

> **Note on manifest "tests":** K8s YAML has no unit test. The TDD cycle here is: (a) write manifest, (b) `kubectl apply --dry-run=server` or `kubeconform` to verify it's valid and accepted by the API server (the "run and watch it fail/pass" step), (c) for behavior, assert via `kubectl wait`/`kubectl get` that the resource reaches the expected state. Each task's "test" step is an explicit `kubectl` assertion with expected output.

---

### Task 1: Namespace + cluster bring-up skeleton

**Files:**
- Create: `infrastructure/kubernetes/namespace.yaml`
- Modify: `Makefile` (add `k8s-create-cluster`, `k8s-delete-cluster`)

**Interfaces:**
- Produces: k3d cluster `image-platform` with Traefik + loadbalancer on host port 8088; namespace `image-platform`; kube context `k3d-image-platform`.

- [ ] **Step 1: Write the namespace manifest**

```yaml
# infrastructure/kubernetes/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: image-platform
```

- [ ] **Step 2: Add cluster lifecycle Makefile targets**

```makefile
# ── Kubernetes (chapter 5) ──────────────────────────────────────────────────
K3D_CLUSTER := image-platform
K8S_DIR     := infrastructure/kubernetes
K8S_NS      := image-platform

k8s-create-cluster: ## Create the k3d cluster (Traefik on localhost:8088)
	k3d cluster create $(K3D_CLUSTER) \
	  --port "8088:80@loadbalancer" \
	  --agents 1 \
	  --wait
	kubectl config use-context k3d-$(K3D_CLUSTER)

k8s-delete-cluster: ## Delete the k3d cluster
	- k3d cluster delete $(K3D_CLUSTER)
```

- [ ] **Step 3: Create the cluster and apply the namespace (the "run" step)**

Run:
```bash
make k8s-create-cluster
kubectl apply -f infrastructure/kubernetes/namespace.yaml
```
Expected: `cluster 'image-platform' created`, then `namespace/image-platform created`.

- [ ] **Step 4: Verify the namespace exists**

Run: `kubectl get ns image-platform -o jsonpath='{.status.phase}'`
Expected: `Active`

- [ ] **Step 5: Commit**

```bash
git add infrastructure/kubernetes/namespace.yaml Makefile
git commit -m "chapter 5: k3d cluster bring-up + image-platform namespace"
```

---

### Task 2: ConfigMaps (non-secret config + DB migrations)

**Files:**
- Create: `infrastructure/kubernetes/configmaps/platform-config.yaml`
- Create: `infrastructure/kubernetes/configmaps/db-migrations.yaml`
- Modify: `Makefile` (add `k8s-configmaps` that regenerates db-migrations from source SQL)

**Interfaces:**
- Produces: ConfigMap `platform-config` (keys below) and ConfigMap `db-migrations` (keys `001_create_images.sql`, `002_create_jobs.sql`) in `image-platform`.

- [ ] **Step 1: Write platform-config.yaml**

```yaml
# infrastructure/kubernetes/configmaps/platform-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: platform-config
  namespace: image-platform
data:
  API_PORT: "8080"
  JOB_SERVICE_PORT: "8081"
  MINIO_ENDPOINT: "minio:8000"
  MINIO_USE_SSL: "false"
  MAX_UPLOAD_SIZE_MB: "10"
  MAX_MEGAPIXELS: "50000000"
  PRESIGNED_URL_TTL_MINUTES: "60"
  OUTBOX_POLL_MS: "500"
  WORKER_LOG_LEVEL: "INFO"
  POSTGRES_DB: "image_platform"
  POSTGRES_USER: "platform"
```

- [ ] **Step 2: Generate db-migrations ConfigMap from the real SQL files**

Run (this is the generator; it produces the manifest so the SQL never drifts):
```bash
kubectl create configmap db-migrations \
  --namespace image-platform \
  --from-file=database/migrations/001_create_images.sql \
  --from-file=database/migrations/002_create_jobs.sql \
  --dry-run=client -o yaml > infrastructure/kubernetes/configmaps/db-migrations.yaml
```
Expected: file written with both SQL files as data keys.

- [ ] **Step 3: Add Makefile target to regenerate it**

```makefile
k8s-configmaps: ## Regenerate the db-migrations ConfigMap from source SQL
	kubectl create configmap db-migrations \
	  --namespace $(K8S_NS) \
	  --from-file=database/migrations/ \
	  --dry-run=client -o yaml > $(K8S_DIR)/configmaps/db-migrations.yaml
```

- [ ] **Step 4: Apply and verify (the "run" step)**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/configmaps/
kubectl get configmap platform-config db-migrations -n image-platform
```
Expected: both ConfigMaps listed; `db-migrations` has `DATA   2`.

- [ ] **Step 5: Verify a key resolves**

Run: `kubectl get configmap platform-config -n image-platform -o jsonpath='{.data.MINIO_ENDPOINT}'`
Expected: `minio:8000`

- [ ] **Step 6: Commit**

```bash
git add infrastructure/kubernetes/configmaps/ Makefile
git commit -m "chapter 5: platform-config + db-migrations ConfigMaps"
```

---

### Task 3: Secret template + gitignore

**Files:**
- Create: `infrastructure/kubernetes/secrets.example/platform-secrets.example.yaml`
- Modify: `.gitignore` (add the real secrets path)
- Modify: `Makefile` (add `k8s-secret` that copies the template to the real, gitignored file on first run)

**Interfaces:**
- Produces: Secret `platform-secrets` with keys `POSTGRES_PASSWORD`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `RABBITMQ_DEFAULT_PASS`, `DATABASE_URL`, `RABBITMQ_URL`, and `RABBITMQ_AMQP_URL` (used by KEDA TriggerAuthentication in Task 10).

- [ ] **Step 1: Write the example secret (stringData → no manual base64)**

```yaml
# infrastructure/kubernetes/secrets.example/platform-secrets.example.yaml
# Copy to infrastructure/kubernetes/secrets.yaml (gitignored) and fill real values.
apiVersion: v1
kind: Secret
metadata:
  name: platform-secrets
  namespace: image-platform
type: Opaque
stringData:
  POSTGRES_PASSWORD: "changeme"
  MINIO_ACCESS_KEY: "accessKey1"
  MINIO_SECRET_KEY: "verySecretKey1"
  RABBITMQ_DEFAULT_PASS: "guest"
  # Composed connection strings (contain credentials → Secret, not ConfigMap).
  DATABASE_URL: "postgres://platform:changeme@postgres:5432/image_platform?sslmode=disable"
  RABBITMQ_URL: "amqp://guest:guest@rabbitmq:5672/"
  # Consumed by KEDA TriggerAuthentication (parameter: host). Same value as RABBITMQ_URL.
  RABBITMQ_AMQP_URL: "amqp://guest:guest@rabbitmq:5672/"
```

- [ ] **Step 2: Add the real secret file to .gitignore**

Append to `.gitignore`:
```
# Real K8s secret (template lives in secrets.example/)
infrastructure/kubernetes/secrets.yaml
```

- [ ] **Step 3: Add Makefile target to materialize + apply the secret**

```makefile
k8s-secret: ## Create the real secret from the template on first run, then apply it
	@test -f $(K8S_DIR)/secrets.yaml || cp $(K8S_DIR)/secrets.example/platform-secrets.example.yaml $(K8S_DIR)/secrets.yaml
	kubectl apply -f $(K8S_DIR)/secrets.yaml
```

- [ ] **Step 4: Apply and verify (the "run" step)**

Run:
```bash
make k8s-secret
kubectl get secret platform-secrets -n image-platform -o jsonpath='{.data.DATABASE_URL}' | base64 -d
```
Expected: `postgres://platform:changeme@postgres:5432/image_platform?sslmode=disable`

- [ ] **Step 5: Verify the real secret is gitignored**

Run: `git status --porcelain infrastructure/kubernetes/secrets.yaml`
Expected: empty output (file is ignored, not shown as untracked).

- [ ] **Step 6: Commit**

```bash
git add infrastructure/kubernetes/secrets.example/ .gitignore Makefile
git commit -m "chapter 5: platform-secrets template + gitignore real secret"
```

---

### Task 4: Postgres Deployment + Service (with migrations)

**Files:**
- Create: `infrastructure/kubernetes/deployments/postgres.yaml`
- Create: `infrastructure/kubernetes/services/postgres.yaml`

**Interfaces:**
- Consumes: ConfigMap `platform-config` (`POSTGRES_DB`, `POSTGRES_USER`), Secret `platform-secrets` (`POSTGRES_PASSWORD`), ConfigMap `db-migrations`.
- Produces: Service `postgres:5432` (ClusterIP) reachable by api-service/job-service/worker.

- [ ] **Step 1: Write the Deployment**

```yaml
# infrastructure/kubernetes/deployments/postgres.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: postgres }
  template:
    metadata:
      labels: { app: postgres }
    spec:
      containers:
        - name: postgres
          image: postgres:16-alpine
          ports:
            - containerPort: 5432
          env:
            - name: POSTGRES_DB
              valueFrom: { configMapKeyRef: { name: platform-config, key: POSTGRES_DB } }
            - name: POSTGRES_USER
              valueFrom: { configMapKeyRef: { name: platform-config, key: POSTGRES_USER } }
            - name: POSTGRES_PASSWORD
              valueFrom: { secretKeyRef: { name: platform-secrets, key: POSTGRES_PASSWORD } }
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
            - name: migrations
              mountPath: /docker-entrypoint-initdb.d
              readOnly: true
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "platform", "-d", "image_platform"]
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: data
          emptyDir: {}
        - name: migrations
          configMap:
            name: db-migrations
```

- [ ] **Step 2: Write the Service**

```yaml
# infrastructure/kubernetes/services/postgres.yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: image-platform
spec:
  selector: { app: postgres }
  ports:
    - port: 5432
      targetPort: 5432
```

- [ ] **Step 3: Server-side dry-run (the "watch it validate" step)**

Run: `kubectl apply --dry-run=server -f infrastructure/kubernetes/deployments/postgres.yaml -f infrastructure/kubernetes/services/postgres.yaml`
Expected: `deployment.apps/postgres created (server dry run)` and `service/postgres created (server dry run)` with no errors.

- [ ] **Step 4: Apply and wait for ready (the "pass" step)**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/deployments/postgres.yaml -f infrastructure/kubernetes/services/postgres.yaml
kubectl wait --for=condition=available deployment/postgres -n image-platform --timeout=90s
```
Expected: `deployment.apps/postgres condition met`.

- [ ] **Step 5: Verify migrations ran (tables exist)**

Run:
```bash
kubectl exec -n image-platform deploy/postgres -- psql -U platform -d image_platform -c "\dt" 2>/dev/null
```
Expected: table list includes `images`, `jobs`, `outbox_events`.

- [ ] **Step 6: Commit**

```bash
git add infrastructure/kubernetes/deployments/postgres.yaml infrastructure/kubernetes/services/postgres.yaml
git commit -m "chapter 5: postgres Deployment + Service with initdb migrations"
```

---

### Task 5: RabbitMQ Deployment + Service

**Files:**
- Create: `infrastructure/kubernetes/deployments/rabbitmq.yaml`
- Create: `infrastructure/kubernetes/services/rabbitmq.yaml`

**Interfaces:**
- Consumes: ConfigMap `platform-config` (none needed), Secret `platform-secrets` (`RABBITMQ_DEFAULT_PASS`).
- Produces: Service `rabbitmq` exposing `5672` (AMQP) and `15672` (management, used by KEDA http protocol fallback + debugging).

- [ ] **Step 1: Write the Deployment**

```yaml
# infrastructure/kubernetes/deployments/rabbitmq.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rabbitmq
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: rabbitmq }
  template:
    metadata:
      labels: { app: rabbitmq }
    spec:
      containers:
        - name: rabbitmq
          image: rabbitmq:3.13-management-alpine
          ports:
            - containerPort: 5672
            - containerPort: 15672
          env:
            - name: RABBITMQ_DEFAULT_USER
              value: "guest"
            - name: RABBITMQ_DEFAULT_PASS
              valueFrom: { secretKeyRef: { name: platform-secrets, key: RABBITMQ_DEFAULT_PASS } }
          readinessProbe:
            exec:
              command: ["rabbitmq-diagnostics", "ping"]
            initialDelaySeconds: 15
            periodSeconds: 10
            timeoutSeconds: 5
```

- [ ] **Step 2: Write the Service**

```yaml
# infrastructure/kubernetes/services/rabbitmq.yaml
apiVersion: v1
kind: Service
metadata:
  name: rabbitmq
  namespace: image-platform
spec:
  selector: { app: rabbitmq }
  ports:
    - name: amqp
      port: 5672
      targetPort: 5672
    - name: management
      port: 15672
      targetPort: 15672
```

- [ ] **Step 3: Dry-run validate**

Run: `kubectl apply --dry-run=server -f infrastructure/kubernetes/deployments/rabbitmq.yaml -f infrastructure/kubernetes/services/rabbitmq.yaml`
Expected: both resources accepted, no errors.

- [ ] **Step 4: Apply and wait**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/deployments/rabbitmq.yaml -f infrastructure/kubernetes/services/rabbitmq.yaml
kubectl wait --for=condition=available deployment/rabbitmq -n image-platform --timeout=120s
```
Expected: `deployment.apps/rabbitmq condition met`.

- [ ] **Step 5: Verify it answers diagnostics**

Run: `kubectl exec -n image-platform deploy/rabbitmq -- rabbitmq-diagnostics ping`
Expected: `Ping succeeded`

- [ ] **Step 6: Commit**

```bash
git add infrastructure/kubernetes/deployments/rabbitmq.yaml infrastructure/kubernetes/services/rabbitmq.yaml
git commit -m "chapter 5: rabbitmq Deployment + Service"
```

---

### Task 6: CloudServer (minio) Deployment + Service

**Files:**
- Create: `infrastructure/kubernetes/deployments/minio.yaml`
- Create: `infrastructure/kubernetes/services/minio.yaml`

**Interfaces:**
- Consumes: nothing (uses image-default credentials — see Global Constraints).
- Produces: Service `minio:8000` (S3 API) for api-service + worker.

- [ ] **Step 1: Write the Deployment (NO credential env — image defaults only)**

```yaml
# infrastructure/kubernetes/deployments/minio.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: minio }
  template:
    metadata:
      labels: { app: minio }
    spec:
      containers:
        - name: cloudserver
          image: zenko/cloudserver:latest-7.70.10
          ports:
            - containerPort: 8000
          env:
            - name: S3BACKEND
              value: "mem"
            - name: REMOTE_MANAGEMENT_DISABLE
              value: "1"
            - name: ENDPOINT
              value: "minio"
          readinessProbe:
            exec:
              command:
                - node
                - -e
                - "require('http').get('http://localhost:8000/',r=>process.exit(0)).on('error',()=>process.exit(1))"
            initialDelaySeconds: 20
            periodSeconds: 5
            timeoutSeconds: 10
            failureThreshold: 30
```

- [ ] **Step 2: Write the Service**

```yaml
# infrastructure/kubernetes/services/minio.yaml
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: image-platform
spec:
  selector: { app: minio }
  ports:
    - port: 8000
      targetPort: 8000
```

- [ ] **Step 3: Dry-run validate**

Run: `kubectl apply --dry-run=server -f infrastructure/kubernetes/deployments/minio.yaml -f infrastructure/kubernetes/services/minio.yaml`
Expected: both accepted.

- [ ] **Step 4: Apply and wait**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/deployments/minio.yaml -f infrastructure/kubernetes/services/minio.yaml
kubectl wait --for=condition=available deployment/minio -n image-platform --timeout=120s
```
Expected: `deployment.apps/minio condition met`.

- [ ] **Step 5: Verify S3 endpoint answers (403 to anon GET proves it's up)**

Run: `kubectl exec -n image-platform deploy/minio -- node -e "require('http').get('http://localhost:8000/',r=>{console.log(r.statusCode);process.exit(0)})"`
Expected: `403`

- [ ] **Step 6: Commit**

```bash
git add infrastructure/kubernetes/deployments/minio.yaml infrastructure/kubernetes/services/minio.yaml
git commit -m "chapter 5: CloudServer (minio) Deployment + Service"
```

---

### Task 7: Build/import images; api-service + job-service Deployments/Services + Ingress

**Files:**
- Create: `infrastructure/kubernetes/deployments/api-service.yaml`
- Create: `infrastructure/kubernetes/deployments/job-service.yaml`
- Create: `infrastructure/kubernetes/services/api-service.yaml`
- Create: `infrastructure/kubernetes/services/job-service.yaml`
- Create: `infrastructure/kubernetes/services/ingress.yaml`
- Modify: `Makefile` (add `k8s-build` image build+import)

**Interfaces:**
- Consumes: `platform-config`, `platform-secrets`, Services `postgres`, `rabbitmq`, `minio`.
- Produces: Services `api-service:8080`, `job-service:8081`; Ingress routing `/` → frontend (frontend added Task 9; Ingress created here so api/job are reachable for their own verification via port-forward meanwhile). api-service runs `EnsureBuckets` on boot → buckets exist for the worker.

- [ ] **Step 1: Add image build + import Makefile target**

```makefile
IMG_TAG := dev
k8s-build: ## Build the four service images and import them into k3d
	docker build -t image-platform/api-service:$(IMG_TAG)        services/api-service
	docker build -t image-platform/job-service:$(IMG_TAG)        services/job-service
	docker build -t image-platform/processing-worker:$(IMG_TAG)  services/processing-worker
	docker build -t image-platform/frontend:$(IMG_TAG)           services/frontend
	k3d image import \
	  image-platform/api-service:$(IMG_TAG) \
	  image-platform/job-service:$(IMG_TAG) \
	  image-platform/processing-worker:$(IMG_TAG) \
	  image-platform/frontend:$(IMG_TAG) \
	  -c $(K3D_CLUSTER)
```

- [ ] **Step 2: Build + import (the "run" step for images)**

Run: `make k8s-build`
Expected: four `naming to docker.io/image-platform/...:dev done` lines, then `Successfully imported image(s)` / `Importing ... into cluster`.

- [ ] **Step 3: Write api-service Deployment (envFrom config + secret, bucket init on boot)**

```yaml
# infrastructure/kubernetes/deployments/api-service.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-service
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: api-service }
  template:
    metadata:
      labels: { app: api-service }
    spec:
      containers:
        - name: api-service
          image: image-platform/api-service:dev
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef: { name: platform-config }
            - secretRef:    { name: platform-secrets }
          readinessProbe:
            httpGet: { path: /healthz, port: 8080 }
            initialDelaySeconds: 3
            periodSeconds: 5
          livenessProbe:
            httpGet: { path: /healthz, port: 8080 }
            initialDelaySeconds: 10
            periodSeconds: 10
```

> `envFrom` injects every ConfigMap/Secret key as an env var. The app reads `DATABASE_URL`, `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL`, `API_PORT`, `MAX_*`, `PRESIGNED_URL_TTL_MINUTES` — all present across the two sources. Extra keys (e.g. `RABBITMQ_URL`) are harmless.

- [ ] **Step 4: Write job-service Deployment**

```yaml
# infrastructure/kubernetes/deployments/job-service.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: job-service
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: job-service }
  template:
    metadata:
      labels: { app: job-service }
    spec:
      containers:
        - name: job-service
          image: image-platform/job-service:dev
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8081
          envFrom:
            - configMapRef: { name: platform-config }
            - secretRef:    { name: platform-secrets }
          readinessProbe:
            httpGet: { path: /healthz, port: 8081 }
            initialDelaySeconds: 3
            periodSeconds: 5
          livenessProbe:
            httpGet: { path: /healthz, port: 8081 }
            initialDelaySeconds: 10
            periodSeconds: 10
```

- [ ] **Step 5: Write api-service + job-service Services**

```yaml
# infrastructure/kubernetes/services/api-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: api-service
  namespace: image-platform
spec:
  selector: { app: api-service }
  ports:
    - port: 8080
      targetPort: 8080
---
# infrastructure/kubernetes/services/job-service.yaml  (separate file)
apiVersion: v1
kind: Service
metadata:
  name: job-service
  namespace: image-platform
spec:
  selector: { app: job-service }
  ports:
    - port: 8081
      targetPort: 8081
```

> Write these as two separate files at the paths named in **Files**.

- [ ] **Step 6: Write the Ingress (→ frontend; frontend added Task 9)**

```yaml
# infrastructure/kubernetes/services/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: platform-ingress
  namespace: image-platform
spec:
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: frontend
                port:
                  number: 80
```

> No `host:` → matches any Host header on the loadbalancer, so `http://localhost:8088` reaches it directly. Routing to api/job happens inside the frontend's nginx.

- [ ] **Step 7: Apply api + job; dry-run the ingress (frontend not up yet)**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/deployments/api-service.yaml \
              -f infrastructure/kubernetes/deployments/job-service.yaml \
              -f infrastructure/kubernetes/services/api-service.yaml \
              -f infrastructure/kubernetes/services/job-service.yaml
kubectl apply --dry-run=server -f infrastructure/kubernetes/services/ingress.yaml
kubectl wait --for=condition=available deployment/api-service deployment/job-service -n image-platform --timeout=120s
```
Expected: both deployments `condition met`; ingress dry-run accepted.

- [ ] **Step 8: Verify api-service ensured buckets (log line) and job-service health**

Run:
```bash
kubectl logs -n image-platform deploy/api-service | grep -i bucket || echo "no bucket error (buckets ensured)"
kubectl exec -n image-platform deploy/job-service -- wget -qO- http://localhost:8081/healthz && echo
```
Expected: no "ensure buckets" error line; healthz returns JSON (e.g. `{"status":"ok"}` or `200` body).

- [ ] **Step 9: Apply the Ingress for real**

Run: `kubectl apply -f infrastructure/kubernetes/services/ingress.yaml`
Expected: `ingress.networking.k8s.io/platform-ingress created`.

- [ ] **Step 10: Commit**

```bash
git add infrastructure/kubernetes/deployments/api-service.yaml \
        infrastructure/kubernetes/deployments/job-service.yaml \
        infrastructure/kubernetes/services/api-service.yaml \
        infrastructure/kubernetes/services/job-service.yaml \
        infrastructure/kubernetes/services/ingress.yaml Makefile
git commit -m "chapter 5: api-service + job-service Deployments/Services, image build+import, Ingress"
```

---

### Task 8: processing-worker Deployment (waits for api-service)

**Files:**
- Create: `infrastructure/kubernetes/deployments/processing-worker.yaml`

**Interfaces:**
- Consumes: `platform-config`, `platform-secrets`, Services `postgres`, `rabbitmq`, `minio`, `api-service` (Ready gate so buckets exist — Review Focus #2).
- Produces: Deployment `processing-worker` (replicas: 1); the KEDA scale target in Task 10. No Service (worker has no HTTP server).

- [ ] **Step 1: Write the Deployment with an init-container that waits for api-service**

```yaml
# infrastructure/kubernetes/deployments/processing-worker.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: processing-worker
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: processing-worker }
  template:
    metadata:
      labels: { app: processing-worker }
    spec:
      initContainers:
        # Buckets are created by api-service's EnsureBuckets on boot; the worker
        # only put_objects and assumes they exist. Gate startup on api-service /healthz.
        - name: wait-for-api
          image: busybox:1.36
          command:
            - sh
            - -c
            - |
              until wget -q -T 2 -O /dev/null http://api-service:8080/healthz; do
                echo "waiting for api-service (buckets)..."; sleep 2;
              done
      containers:
        - name: processing-worker
          image: image-platform/processing-worker:dev
          imagePullPolicy: IfNotPresent
          envFrom:
            - configMapRef: { name: platform-config }
            - secretRef:    { name: platform-secrets }
```

- [ ] **Step 2: Dry-run validate**

Run: `kubectl apply --dry-run=server -f infrastructure/kubernetes/deployments/processing-worker.yaml`
Expected: accepted.

- [ ] **Step 3: Apply and wait**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/deployments/processing-worker.yaml
kubectl wait --for=condition=available deployment/processing-worker -n image-platform --timeout=120s
```
Expected: `condition met` (implies the init-container passed → api-service was Ready).

- [ ] **Step 4: Verify the worker connected to RabbitMQ (log line)**

Run: `kubectl logs -n image-platform deploy/processing-worker | tail -5`
Expected: a "processing-worker starting" log line and no stack trace / connection-refused loop.

- [ ] **Step 5: Commit**

```bash
git add infrastructure/kubernetes/deployments/processing-worker.yaml
git commit -m "chapter 5: processing-worker Deployment (init-container waits for api-service buckets)"
```

---

### Task 9: Frontend Deployment + Service (waits for api/job)

**Files:**
- Create: `infrastructure/kubernetes/deployments/frontend.yaml`
- Create: `infrastructure/kubernetes/services/frontend.yaml`

**Interfaces:**
- Consumes: Services `api-service`, `job-service` (Ready gate — Review Focus #1, nginx resolves upstreams at boot).
- Produces: Service `frontend:80`, the Ingress backend.

- [ ] **Step 1: Write the Deployment with init-container waiting for both upstreams**

```yaml
# infrastructure/kubernetes/deployments/frontend.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: image-platform
spec:
  replicas: 1
  selector:
    matchLabels: { app: frontend }
  template:
    metadata:
      labels: { app: frontend }
    spec:
      initContainers:
        # nginx resolves proxy_pass hostnames once at startup (no resolver directive).
        # Block until both upstream Services answer so nginx never caches NXDOMAIN.
        - name: wait-for-upstreams
          image: busybox:1.36
          command:
            - sh
            - -c
            - |
              until wget -q -T 2 -O /dev/null http://api-service:8080/healthz; do
                echo "waiting for api-service..."; sleep 2; done
              until wget -q -T 2 -O /dev/null http://job-service:8081/healthz; do
                echo "waiting for job-service..."; sleep 2; done
      containers:
        - name: frontend
          image: image-platform/frontend:dev
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet: { path: /healthz, port: 80 }
            initialDelaySeconds: 3
            periodSeconds: 5
```

- [ ] **Step 2: Write the Service**

```yaml
# infrastructure/kubernetes/services/frontend.yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: image-platform
spec:
  selector: { app: frontend }
  ports:
    - port: 80
      targetPort: 80
```

- [ ] **Step 3: Dry-run validate**

Run: `kubectl apply --dry-run=server -f infrastructure/kubernetes/deployments/frontend.yaml -f infrastructure/kubernetes/services/frontend.yaml`
Expected: both accepted.

- [ ] **Step 4: Apply and wait**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/deployments/frontend.yaml -f infrastructure/kubernetes/services/frontend.yaml
kubectl wait --for=condition=available deployment/frontend -n image-platform --timeout=120s
```
Expected: `condition met`.

- [ ] **Step 5: Verify the frontend is reachable through the Ingress (end-to-end routing)**

Run: `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8088/healthz`
Expected: `200` (nginx proxied `/healthz` → api-service through the cluster, via Ingress → frontend).

- [ ] **Step 6: Commit**

```bash
git add infrastructure/kubernetes/deployments/frontend.yaml infrastructure/kubernetes/services/frontend.yaml
git commit -m "chapter 5: frontend Deployment + Service (init-container waits for upstreams)"
```

---

### Task 10: KEDA install + RabbitMQ queue-depth autoscaling

**Files:**
- Create: `infrastructure/kubernetes/autoscaling/trigger-auth.yaml`
- Create: `infrastructure/kubernetes/autoscaling/keda-scaledobject.yaml`
- Modify: `Makefile` (add `k8s-keda` install target)

**Interfaces:**
- Consumes: Secret `platform-secrets` key `RABBITMQ_AMQP_URL`, Deployment `processing-worker`.
- Produces: a KEDA-managed HPA scaling `processing-worker` 1→8 on queue `jobs` length.

- [ ] **Step 1: Add KEDA + metrics-server install Makefile target**

```makefile
KEDA_VERSION := 2.21.1
k8s-keda: ## Install KEDA and metrics-server into the cluster
	kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v$(KEDA_VERSION)/keda-$(KEDA_VERSION).yaml
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	# k3d/k3s kubelet serving cert is self-signed; allow metrics-server to trust it.
	kubectl patch -n kube-system deployment metrics-server --type=json \
	  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
	kubectl wait --for=condition=available deployment/keda-operator -n keda --timeout=180s
```

> The release **tag** carries a `v` prefix (`v2.21.1`) but the asset **filename** does not (`keda-2.21.1.yaml`) — the URL above splits them correctly. `KEDA_VERSION` holds the bare number.

- [ ] **Step 2: Install KEDA (the "run" step)**

Run: `make k8s-keda`
Expected: KEDA CRDs + operator applied; `deployment.apps/keda-operator condition met`.

- [ ] **Step 3: Verify the ScaledObject CRD is registered**

Run: `kubectl get crd scaledobjects.keda.sh -o jsonpath='{.metadata.name}'`
Expected: `scaledobjects.keda.sh`

- [ ] **Step 4: Write the TriggerAuthentication (reads AMQP URL from the Secret)**

```yaml
# infrastructure/kubernetes/autoscaling/trigger-auth.yaml
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata:
  name: keda-trigger-auth-rabbitmq
  namespace: image-platform
spec:
  secretTargetRef:
    - parameter: host
      name: platform-secrets
      key: RABBITMQ_AMQP_URL
```

- [ ] **Step 5: Write the ScaledObject**

```yaml
# infrastructure/kubernetes/autoscaling/keda-scaledobject.yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: processing-worker-scaler
  namespace: image-platform
spec:
  scaleTargetRef:
    name: processing-worker
  minReplicaCount: 1
  maxReplicaCount: 8
  pollingInterval: 5      # seconds between queue-length checks
  cooldownPeriod: 30      # seconds before scaling back down (short, for experiments)
  triggers:
    - type: rabbitmq
      metadata:
        protocol: amqp
        queueName: jobs
        mode: QueueLength
        value: "5"          # ~5 queued msgs per replica
      authenticationRef:
        name: keda-trigger-auth-rabbitmq
```

- [ ] **Step 6: Apply and verify the ScaledObject is Ready**

Run:
```bash
kubectl apply -f infrastructure/kubernetes/autoscaling/trigger-auth.yaml -f infrastructure/kubernetes/autoscaling/keda-scaledobject.yaml
kubectl get scaledobject processing-worker-scaler -n image-platform -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'; echo
```
Expected: `True` (KEDA accepted the trigger + auth; a generated HPA now exists: `kubectl get hpa -n image-platform` shows `keda-hpa-processing-worker-scaler`).

- [ ] **Step 7: Commit**

```bash
git add infrastructure/kubernetes/autoscaling/ Makefile
git commit -m "chapter 5: KEDA install + RabbitMQ queue-depth ScaledObject for processing-worker"
```

---

### Task 11: `make k8s-up` / `k8s-down` / `k8s-status` orchestration

**Files:**
- Modify: `Makefile` (compose the one-shot `k8s-up`, plus `k8s-down`, `k8s-status`, `k8s-logs`)

**Interfaces:**
- Consumes: all targets from Tasks 1–10.
- Produces: `make k8s-up` brings the whole stack from nothing to Ready in dependency order.

- [ ] **Step 1: Add the orchestration targets**

```makefile
k8s-up: k8s-create-cluster k8s-keda k8s-build ## Full bring-up: cluster → KEDA → images → manifests
	kubectl apply -f $(K8S_DIR)/namespace.yaml
	kubectl apply -f $(K8S_DIR)/configmaps/
	$(MAKE) k8s-secret
	kubectl apply -f $(K8S_DIR)/deployments/postgres.yaml -f $(K8S_DIR)/services/postgres.yaml
	kubectl apply -f $(K8S_DIR)/deployments/rabbitmq.yaml -f $(K8S_DIR)/services/rabbitmq.yaml
	kubectl apply -f $(K8S_DIR)/deployments/minio.yaml    -f $(K8S_DIR)/services/minio.yaml
	kubectl wait --for=condition=available deployment/postgres deployment/rabbitmq deployment/minio -n $(K8S_NS) --timeout=180s
	kubectl apply -f $(K8S_DIR)/deployments/api-service.yaml -f $(K8S_DIR)/services/api-service.yaml
	kubectl apply -f $(K8S_DIR)/deployments/job-service.yaml -f $(K8S_DIR)/services/job-service.yaml
	kubectl wait --for=condition=available deployment/api-service deployment/job-service -n $(K8S_NS) --timeout=180s
	kubectl apply -f $(K8S_DIR)/deployments/processing-worker.yaml
	kubectl apply -f $(K8S_DIR)/deployments/frontend.yaml -f $(K8S_DIR)/services/frontend.yaml
	kubectl apply -f $(K8S_DIR)/services/ingress.yaml
	kubectl apply -f $(K8S_DIR)/autoscaling/
	kubectl wait --for=condition=available deployment/frontend deployment/processing-worker -n $(K8S_NS) --timeout=180s
	@echo "Platform up → http://localhost:8088"

k8s-down: k8s-delete-cluster ## Tear down the whole cluster

k8s-status: ## Show pods, services, HPA, ScaledObject
	kubectl get pods,svc,hpa,scaledobject -n $(K8S_NS)

k8s-logs: ## Tail processing-worker logs
	kubectl logs -f -n $(K8S_NS) deploy/processing-worker
```

- [ ] **Step 2: Full clean bring-up (the real integration "test")**

Run:
```bash
make k8s-down 2>/dev/null; make k8s-up
```
Expected: ends with `Platform up → http://localhost:8088` and no failed `kubectl wait`.

- [ ] **Step 3: Verify every workload is Ready**

Run: `make k8s-status`
Expected: all pods `Running`/`1/1` (worker may be `1/1`), `scaledobject` READY `True`, `hpa` present.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chapter 5: make k8s-up/down/status/logs orchestration"
```

---

### Task 12: Kubernetes end-to-end smoke test

**Files:**
- Create: `scripts/k8s-smoke.sh`
- Modify: `Makefile` (add `k8s-smoke`)

**Interfaces:**
- Consumes: a running cluster from `make k8s-up`; the Ingress on `http://localhost:8088`.
- Produces: pass/fail assertion of upload → create job → poll COMPLETED → **download result 200 + non-empty** (closes the spec's smoke-test gap, Review Focus #1 routing + #5 ingress reachability).

- [ ] **Step 1: Write the smoke script (all traffic via the Ingress on one base URL)**

```bash
#!/usr/bin/env bash
# K8s end-to-end smoke test — everything goes through the Ingress (frontend nginx routes internally).
# Usage: BASE_URL=http://localhost:8088 bash scripts/k8s-smoke.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8088}"
ok(){ echo "  ✓ $*"; }; fail(){ echo "  ✗ $*" >&2; exit 1; }

echo "=== K8s smoke → ${BASE_URL} ==="

# 0. health (nginx → api-service)
[ "$(curl -s -o /dev/null -w '%{http_code}' "${BASE_URL}/healthz")" = "200" ] \
  && ok "healthz 200" || fail "healthz not 200"

# 1. make a test JPEG
IMG=$(mktemp /tmp/k8s_smoke_XXXXXX.jpg); trap 'rm -f "$IMG"' EXIT
python3 -c "from PIL import Image; Image.new('RGB',(200,150),(180,80,40)).save('$IMG','JPEG')"

# 2. upload → image id
UP=$(curl -sf -X POST "${BASE_URL}/api/v1/images" -F "image=@${IMG}")
IMAGE_ID=$(echo "$UP" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
ok "uploaded image ${IMAGE_ID}"

# 3. create job
JOB=$(curl -sf -X POST "${BASE_URL}/api/v1/jobs" -H 'Content-Type: application/json' \
  -d '{"image_id":"'"$IMAGE_ID"'","operation":"resize","parameters":{"width":800}}')
JOB_ID=$(echo "$JOB" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
ok "created job ${JOB_ID}"

# 4. poll for COMPLETED (worker must process it → proves queue + worker + buckets)
for i in $(seq 1 30); do
  ST=$(curl -sf "${BASE_URL}/api/v1/jobs/${JOB_ID}" | python3 -c "import sys,json;print(json.load(sys.stdin)['status'])")
  [ "$ST" = "COMPLETED" ] && break
  [ "$ST" = "FAILED" ] && fail "job FAILED"
  sleep 1
done
[ "$ST" = "COMPLETED" ] || fail "job not COMPLETED (last=$ST)"
ok "job COMPLETED"

# 5. download the RESULT via the /result route (the gap the old smoke test never asserted)
RST=$(curl -s -o /tmp/k8s_result.out -w '%{http_code}' "${BASE_URL}/api/v1/jobs/${JOB_ID}/result")
[ "$RST" = "200" ] || fail "result GET not 200 (got $RST)"
[ -s /tmp/k8s_result.out ] || fail "result body empty"
ok "result downloaded 200, $(wc -c </tmp/k8s_result.out) bytes"
rm -f /tmp/k8s_result.out

echo "=== PASS ==="
```

- [ ] **Step 2: Add the Makefile target**

```makefile
k8s-smoke: ## End-to-end smoke test through the Ingress
	BASE_URL=http://localhost:8088 bash scripts/k8s-smoke.sh
```

- [ ] **Step 3: Run it against the live cluster (the "pass" step)**

Run: `chmod +x scripts/k8s-smoke.sh && make k8s-smoke`
Expected: all `✓` lines ending in `=== PASS ===`, including "result downloaded 200, N bytes" with N>0.

- [ ] **Step 4: Commit**

```bash
git add scripts/k8s-smoke.sh Makefile
git commit -m "chapter 5: k8s end-to-end smoke test (asserts result download, closes smoke gap)"
```

---

### Task 13: Scaling experiment script + results doc

**Files:**
- Create: `scripts/scale-test.sh`
- Create: `docs/results/chapter-5-scaling.md`
- Modify: `Makefile` (add `scale-test`)

**Interfaces:**
- Consumes: a running cluster; the Ingress base URL; `kubectl` to pin replicas.
- Produces: throughput numbers at 1/2/4 workers → written into the results doc.

- [ ] **Step 1: Write the scaling-experiment script**

```bash
#!/usr/bin/env bash
# Scaling experiment: submit a fixed batch of jobs at pinned worker counts, measure throughput.
# Pins replicas by temporarily setting KEDA min=max so the run is controlled.
# Usage: BASE_URL=http://localhost:8088 BATCH=60 bash scripts/scale-test.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8088}"
NS="image-platform"
BATCH="${BATCH:-60}"
SCALER="processing-worker-scaler"

pin() {  # pin worker replicas to $1 via the ScaledObject bounds
  kubectl patch scaledobject "$SCALER" -n "$NS" --type=merge \
    -p "{\"spec\":{\"minReplicaCount\":$1,\"maxReplicaCount\":$1}}"
  kubectl wait --for=jsonpath='{.status.readyReplicas}'=$1 deployment/processing-worker -n "$NS" --timeout=120s
}

submit_batch() {  # upload 1 image, then create $BATCH jobs against it; return nothing
  local img id
  img=$(mktemp /tmp/scale_XXXXXX.jpg)
  python3 -c "from PIL import Image; Image.new('RGB',(1200,900),(120,60,30)).save('$img','JPEG')"
  id=$(curl -sf -X POST "${BASE_URL}/api/v1/images" -F "image=@${img}" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
  rm -f "$img"
  for _ in $(seq 1 "$BATCH"); do
    curl -sf -X POST "${BASE_URL}/api/v1/jobs" -H 'Content-Type: application/json' \
      -d '{"image_id":"'"$id"'","operation":"resize","parameters":{"width":400}}' >/dev/null
  done
}

count_completed() {
  kubectl exec -n "$NS" deploy/postgres -- psql -U platform -d image_platform -tAc \
    "SELECT count(*) FROM jobs WHERE status='COMPLETED';"
}

echo "replicas,batch,seconds,throughput_per_s,speedup" > /tmp/scale_results.csv
BASELINE=""
for N in 1 2 4; do
  echo "=== pinning $N worker(s) ==="
  # fresh DB state for a clean measurement
  kubectl exec -n "$NS" deploy/postgres -- psql -U platform -d image_platform -c \
    "DELETE FROM jobs; DELETE FROM outbox_events;" >/dev/null
  pin "$N"
  START=$(date +%s)
  submit_batch
  # wait until all BATCH jobs are COMPLETED
  while [ "$(count_completed | tr -d '[:space:]')" -lt "$BATCH" ]; do sleep 1; done
  END=$(date +%s)
  SECS=$((END-START)); [ "$SECS" -eq 0 ] && SECS=1
  TPUT=$(python3 -c "print(round($BATCH/$SECS,2))")
  [ -z "$BASELINE" ] && BASELINE="$TPUT"
  SPEEDUP=$(python3 -c "print(round($TPUT/$BASELINE,2))")
  echo "$N,$BATCH,$SECS,$TPUT,$SPEEDUP" | tee -a /tmp/scale_results.csv
done

echo ""
echo "Results CSV:"; cat /tmp/scale_results.csv
echo ""
echo "Restoring autoscaling bounds (min=1,max=8)…"
kubectl patch scaledobject "$SCALER" -n "$NS" --type=merge \
  -p '{"spec":{"minReplicaCount":1,"maxReplicaCount":8}}'
```

- [ ] **Step 2: Add the Makefile target**

```makefile
scale-test: ## Run the 1/2/4-worker scaling experiment (writes /tmp/scale_results.csv)
	BASE_URL=http://localhost:8088 bash scripts/scale-test.sh
```

- [ ] **Step 3: Run the experiment (the "run" step)**

Run: `chmod +x scripts/scale-test.sh && make scale-test`
Expected: a CSV with 3 rows (replicas 1,2,4), each with a seconds + throughput value; throughput increases (not necessarily linearly) with replica count.

- [ ] **Step 4: Write the results doc with the measured numbers**

Create `docs/results/chapter-5-scaling.md`. Paste the actual CSV into the table and write the analysis. Template:

```markdown
# Chapter 5 — Worker Scaling Experiment (M6)

## Setup
- Cluster: k3d v5.9 (k8s v1.36), single node, Rancher Desktop on macOS.
- Autoscaling: KEDA RabbitMQ scaler on queue `jobs`, pinned to a fixed replica count per run.
- Workload: <BATCH> identical `resize width=400` jobs on a 1200×900 JPEG, submitted back-to-back.
- Measurement: wall-clock from first job submitted to the <BATCH>th job reaching COMPLETED (polled from Postgres).
- Each run starts from an empty `jobs` table.

## Results

| Workers | Jobs | Time (s) | Throughput (jobs/s) | Speedup |
|--------:|-----:|---------:|--------------------:|--------:|
| 1 | <B> | <t1> | <x1> | 1.00 |
| 2 | <B> | <t2> | <x2> | <s2> |
| 4 | <B> | <t4> | <x4> | <s4> |

## Analysis
- <Scaling from 1→2 gives ~<s2>× speedup; 1→4 gives ~<s4>×.>
- <Where it departs from linear and why: single-node CPU saturation / shared Postgres + CloudServer / per-job fixed overhead.>
- **Why `prefetch_count=1` matters:** each worker holds exactly one message, so added replicas pull from the shared queue evenly — no single worker hoards the backlog. This is what makes horizontal scaling translate into throughput.
- **Autoscaling behavior (observed, unpinned):** with the ScaledObject live, submitting a burst drives the queue above `value=5`/replica and KEDA scales toward maxReplicas; after the queue drains, it returns to 1 within ~cooldownPeriod.
- **Caveat:** `S3BACKEND=mem` + ephemeral Postgres mean a CloudServer or Postgres pod restart mid-run invalidates the run; each measurement here is a fresh clean run.

## Conclusion
<1–2 sentences: the platform scales ~linearly to the point of single-node resource saturation, validating the queue-based, prefetch_count=1 design for the thesis.>
```

Fill every `<…>` placeholder with the real values from Step 3 before committing.

- [ ] **Step 5: Verify the doc has no remaining placeholders**

Run: `grep -nE "<[A-Za-z0-9]|TBD|TODO" docs/results/chapter-5-scaling.md || echo "clean"`
Expected: `clean`

- [ ] **Step 6: Commit**

```bash
git add scripts/scale-test.sh docs/results/chapter-5-scaling.md Makefile
git commit -m "chapter 5: scaling experiment script + measured results doc (M6)"
```

---

### Task 14: Update CLAUDE.md chapter status + docs

**Files:**
- Modify: `CLAUDE.md` (mark Chapter 5 DONE; add k8s commands)

**Interfaces:**
- Consumes: nothing.
- Produces: accurate chapter-status table and command list.

- [ ] **Step 1: Flip Chapter 5 to DONE and add commands**

In `CLAUDE.md`, change the Chapter 5 row status from `⬜ TODO` to `✅ DONE`, and add under Commands:
```
make k8s-up       # create k3d cluster, install KEDA, build+import images, deploy everything
make k8s-down     # delete the k3d cluster
make k8s-status   # pods / services / HPA / ScaledObject
make k8s-smoke    # end-to-end smoke test through the Ingress
make scale-test   # 1/2/4-worker scaling experiment
```

- [ ] **Step 2: Verify the whole testing gate still passes**

Run: `make test-unit && make build && make lint`
Expected: all green (no app code changed).

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "chapter 5: mark DONE, document k8s commands"
```

---

## Self-Review

**1. Spec coverage** — every spec section maps to a task:
- Namespace/cluster → T1; ConfigMaps → T2; Secret+gitignore → T3; postgres → T4; rabbitmq → T5; minio → T6; api/job + Ingress + image build → T7; worker (+bucket-order fix) → T8; frontend (+nginx-resolver fix) → T9; KEDA ScaledObject + TriggerAuth → T10; `k8s-up/down/status/logs` → T11; k8s smoke (+result-download gap) → T12; scale experiment + results doc → T13; CLAUDE.md status → T14. No orphan spec sections.

**2. Placeholder scan** — the only `<…>` placeholders are in the results-doc template (T13), which is explicitly a fill-from-real-output template with a grep gate (T13 Step 5) forbidding them at commit. No "TBD"/"add error handling"/"similar to Task N" anywhere.

**3. Type/name consistency** — Service names (`postgres`, `rabbitmq`, `minio`, `api-service`, `job-service`, `frontend`), image tags (`image-platform/<svc>:dev`), queue name (`jobs`), Secret keys (`RABBITMQ_AMQP_URL` used by both T3 and T10's `key:`), ScaledObject name (`processing-worker-scaler` in T10 and T13), and Ingress port (8088) are consistent across all tasks.

**4. Review Focus** — all five items are pinned: #1 nginx-resolve → T9 init-container + T12 end-to-end via fresh `k8s-up`; #2 bucket-order → T8 init-container; #3 KEDA secret format → T3 `stringData` + T10 `parameter: host`; #4 CloudServer restart → documented in T13; #5 ingress reachability → T7/T9 loadbalancer port + T12 `localhost:8088`.

**Flagged for implementer:** KEDA version is pinned at `2.21.1` in T10 (verified live: the `v2.21.1` release and its `keda-2.21.1.yaml` asset both resolve). If a later release is wanted, update `KEDA_VERSION` to the bare number — the tag path adds the `v`, the filename does not. Everything else was verified against the live tooling (k3d v5.9, k8s v1.36, kubectl v1.37) in this session.
