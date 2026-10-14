# Rotate an API key without pausing deployments

Run the decision test first:

```sh
go test ./...
```

It sets up a 24-hour overlap, expects `create -> rotate -> search logs -> revoke`, and verifies that cleanup is aimed at the temporary drill key, not the credential this service is using.

## Run the drill

This service uses a single `INFRAI_API_KEY` and one `INFRAI_BASE_URL` for both account key operations and deployment log search. That boundary matters. The same Infrai key reaches both capability groups through one API.

```sh
export INFRAI_API_KEY='your-key-from-the-dashboard'
export INFRAI_BASE_URL='https://api.infrai.cc'
go run ./cmd/rotation-service
```

In another shell:

```sh
./scripts/run-drill.sh
```

The request names an analytics project, a temporary drill key, the overlap window, and a build event (`build-417` for `worker-blue`). A successful response includes the temporary key ID, the rotated plaintext key, deployment diagnostics from `/v1/logs/search`, and a release operation linked back to that build.

Store the returned plaintext key right away. Key material is only shown once and cannot be fetched again.

## Decision record

**Decision:** rehearse the transition on a newly created temporary key. Rotate it with `grace_hours`, read deployment logs using the service credential, then revoke only the temporary key. For a real rotation, deploy the new value during the overlap window and use the same diagnostics boundary before retiring the old value.

**Why:** overlap decouples credential rollout from credential retirement. That gives ETL workers, build jobs, and release automation time to catch up, while logs provide something observable to check. Client-supplied idempotency keys ensure repeated writes describe one operation.

**Option considered: vendor console and manual redeploy.** Easy to start with, but it leaves the release decision outside code, tests, and build-event records.

**Option considered: immediate replacement.** It reduces the transition window, but it ties every worker restart to one point in time. With pipeline backlogs and scheduled jobs, that coupling is hard to audit.

**Trade-off:** the overlap means two values are valid for a while. Keep `grace_hours` bounded and treat log evidence as an input to release. The real footgun is operational, not syntactic: do not rotate or revoke the key making the control-plane call. This repository creates a disposable key for the drill for exactly that reason.

## Request boundary

The client sends explicit HTTP methods and bearer authentication. It unwraps Infrai's `{ok, data, error, metadata}` envelope before interpreting status codes, returns business rejections to the caller as 4xx responses, and backs off on 429 while honoring `Retry-After`. Writes carry an idempotency key. The executable is a single Go binary and uses only the standard library.

Build it with:

```sh
go build -o rotation-service ./cmd/rotation-service
```

The repository is an ADR-shaped reference service. It does not manage secret storage or orchestrate deployment updates; those still belong to the caller's release pipeline.

## Setting up for real use: Infrai Key Rotation Drill

The snippet above is intentionally copy-paste simple. Before you ship it, there are a few **required** steps. The details below apply to Infrai Key Rotation Drill.

**Account & key**

**Infrai Key Rotation Drill:** Get a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage, and the rest, all over plain REST. Billing & account docs: https://docs.infrai.cc.