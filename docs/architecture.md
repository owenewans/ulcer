# architecture

Ulcer is one control plane with two Core roles and a replaceable UI.

```text
browser -> Caddy -> UI -> REST + SSE -> HOST -> gRPC + mTLS -> INSTANCE -> engine
                                         |                         |
                                      Badger                    Badger
                                      durable                  in-memory
```

`HOST` owns desired state, identities, subscriptions, billing state, frozen
artifacts and the durable traffic ledger. `INSTANCE` owns one isolated engine,
its rendered configuration, local counters and immediate quota enforcement.
The UI has no privileged back channel: every operation is reproducible through
the public REST API.

## state contract

- Every instance has an opaque UUID, monotonic generation and canonical spec digest.
- A generation can never refer to two different digests.
- HOST sends a complete current snapshot after every reconnect.
- INSTANCE persists the snapshot before acknowledging it, then reconciles toward it.
- Lifecycle is level-triggered desired state; one-shot operations use idempotent command IDs.
- Status reports include applied generation, digest, phase, readiness and failure reason.
- Artifact tags and commits are author input only. Runtime identity is an immutable digest.

## traffic contract

INSTANCE converts cumulative engine counters into deltas and stores them before
transmission. Meter events are identified by `(instance, principal, quota epoch,
sequence)`. HOST applies them idempotently and acknowledges the highest
contiguous sequence. Unacknowledged events stay in the INSTANCE WAL.

eBPF is authoritative for process, cgroup, listener and socket totals. It cannot
recover an authenticated user hidden inside shared encrypted or multiplexed
traffic. Per-user limits are enabled only when an adapter exposes native user
statistics, an instrumented stream wrapper, or explicit one-user isolation.

## isolation

- HOST is the only service allowed to mutate the `inet ulcer` nftables table.
- Engines run as non-root Podman containers with no Podman socket or `NET_ADMIN`.
- Each engine has a private filesystem, cgroup, secrets and control socket.
- One process may contain several inbounds/transports. Three transport profiles
  do not imply three processes.
- Engine control APIs bind to a Unix socket or private loopback interface.

## terminology

- **instance**: one reconciled engine runtime and its adapter.
- **inbound**: one protocol listener inside an instance.
- **endpoint**: an advertised address, independent from physical placement.
- **principal**: a proxy identity whose credentials, limits and usage are managed.
- **squad**: a reusable set of principals, endpoints or routing policy.
- **prober**: a remote observer that performs a real handshake through a named interface.
