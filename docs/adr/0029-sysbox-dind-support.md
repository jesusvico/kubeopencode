# ADR 0029: Container-in-Container (DinD) Support for Agent Workloads

## Status

Accepted

## Date

2026-04-05

## Context

KubeOpenCode runs AI agents as containers on Kubernetes. Some workloads require the ability to run Docker, Podman, or Kind clusters inside these containers — for example:

- **E2E testing**: Agent tasks that need a Kind cluster to run integration/E2E tests against Kubernetes APIs
- **Container builds**: Agent tasks that build and push container images
- **Development environments**: Developers using KubeOpenCode as a cloud dev environment who need local container runtime access

The fundamental challenge is that Kind works by running Kubernetes nodes as Docker containers, which means the agent container must behave like a lightweight VM capable of running nested containers with system-level processes (kubelet, containerd).

This ADR evaluates all viable approaches and recommends a phased adoption strategy.

## Options Evaluated

### Option 1: Docker-in-Docker with Privileged Mode

**How it works:** Run the official `docker:dind` image (or install Docker daemon) inside a container with `--privileged` flag.

```bash
docker run --privileged -d docker:dind
```

**Pros:**
- Simplest setup — works out of the box
- Fully compatible with Kind (~30s cluster startup)
- No node-level changes required
- Well-documented, widely used in CI/CD (GitLab CI, Drone, etc.)
- [bsycorp/kind](https://github.com/bsycorp/kind) provides optimized images for CI

**Cons:**
- **Security: unacceptable for multi-tenant** — `--privileged` grants all host capabilities, container root = host root
- Storage driver issues — overlay2 cannot nest on overlay2, falls back to vfs (significant performance penalty)
- Violates most enterprise security policies (PodSecurityStandards, OPA/Gatekeeper)

**Recommendation:** Short-term only, for trusted single-tenant dev/test environments.

### Option 2: Sysbox Runtime (Recommended Default)

**How it works:** [Sysbox](https://github.com/nestybox/sysbox) is an enhanced OCI runtime (replaces runc) developed by Nestybox, acquired by Docker in 2022. It uses Linux user namespaces and procfs/sysfs virtualization to make containers behave like lightweight VMs — **without `--privileged`**.

```bash
docker run --runtime=sysbox-runc nestybox/ubuntu-focal-systemd-docker
```

**Kubernetes integration:**

1. Install Sysbox on cluster nodes (DaemonSet-based installer available)
2. Create a RuntimeClass:
   ```yaml
   apiVersion: node.k8s.io/v1
   kind: RuntimeClass
   metadata:
     name: sysbox-runc
   handler: sysbox-runc
   ```
3. Agent pods specify `runtimeClassName: sysbox-runc` via `podSpec`

**Pros:**
- **No `--privileged` needed** — container root mapped to unprivileged host user
- Near-native performance (no VM overhead)
- Official Kind support — Nestybox maintains [kindbox](https://github.com/nestybox/kindbox) tool
- Docker-backed project with ongoing investment
- Supports Docker, Kind, K3s, and systemd inside the container
- OCI-compatible — works with containerd, CRI-O

**Cons:**
- Requires cluster administrator to install Sysbox runtime on nodes
- CRI-O officially supported; containerd support is community-maintained
- Not available on all managed Kubernetes services (depends on node access)
- Isolation weaker than hardware virtualization (Kata)
- Community reports occasional hard-to-debug issues

**Recommendation:** Primary recommended approach for medium-term adoption.

### Option 3: Rootless Podman

**How it works:** [Podman](https://podman.io/) is daemonless and runs rootless by default. Running Podman inside a container is more natural than Docker since there is no daemon to manage.

```yaml
# Kubernetes Pod
securityContext:
  runAsUser: 1000
```

**Storage options:**
- **fuse-overlayfs**: User-space filesystem, needs `/dev/fuse`; ~50% performance of kernel overlay
- **Native overlay (recommended)**: Linux 5.13+ supports rootless native overlay; near-rootful performance. [CERN uses this in production](https://kubernetes.web.cern.ch/blog/2025/06/19/rootless-container-builds-on-kubernetes/)

**Pros:**
- **Default rootless** — strongest security posture among container-native options
- No daemon — process directly forked, lower resource consumption
- Fewer kernel capabilities required (11 vs Docker's 14)
- Native SELinux/AppArmor integration
- **Red Hat ecosystem** — first-class support in RHEL, OpenShift; strong enterprise backing
- No node-level runtime installation required
- Apache 2.0 licensed

**Cons:**
- Rootless networking via slirp4netns has slight performance overhead
- docker-compose compatibility not 100% (podman-compose)
- May need user namespace configuration in the cluster

**Kind compatibility:**
- Kind 0.11.0+ supports Podman as node provider (`KIND_EXPERIMENTAL_PROVIDER=podman`)
- Requires cgroup v2 (Ubuntu 21.10+, Fedora 31+, RHEL 9+)
- PID limits need tuning (Podman defaults may be too low)
- Not all Kind features are fully tested with Podman

**Recommendation:** Strong option for Red Hat / OpenShift environments. Best choice when enterprise customers require RHEL-ecosystem alignment or cannot install custom runtimes.

### Option 4: Coder Envbox (Sysbox Wrapper)

**How it works:** [Envbox](https://github.com/coder/envbox) packages the Sysbox runtime inside a wrapper container. The outer container manages Sysbox; the inner unprivileged container is the user workspace. **No node-level runtime installation needed.**

**Pros:**
- No Sysbox installation on nodes — self-contained
- Production-proven (Coder platform)
- Users cannot access the outer management container
- Supports GPU passthrough

**Cons:**
- Coupled to the Coder platform architecture
- Extra abstraction layer adds complexity
- Outer container still runs privileged (but isolated from user)

**Recommendation:** Good reference architecture. Consider if KubeOpenCode needs a similar self-contained approach in the future.

### Option 5: Kata Containers / MicroVM

**How it works:** [Kata Containers](https://katacontainers.io/) runs each container/pod in a dedicated lightweight VM using hardware virtualization (Intel VT-x / AMD-V). [Firecracker](https://firecracker-microvm.github.io/) is a popular VMM choice (~125ms startup).

**Pros:**
- **Strongest isolation** — hardware virtualization boundary; VM escape requires hypervisor CVE
- OCI-compatible — integrates with Kubernetes via RuntimeClass
- Production-proven at scale (Northflank: 2M+ microVMs/month; AWS Lambda uses Firecracker)

**Cons:**
- Requires bare metal nodes or nested virtualization support
- Nested virtualization unstable on some cloud providers
- Higher operational complexity
- VM overhead (memory, startup time)

**Recommendation:** Long-term option for enterprise multi-tenant scenarios requiring hardware-level isolation.

### Option 6: vind (vCluster in Docker)

**How it works:** [vind](https://github.com/loft-sh/vind) is a Kind alternative that runs Kubernetes control planes using vCluster technology inside Docker containers.

**Pros:**
- Supports Service type LoadBalancer (Kind does not)
- Built-in sleep/wake for resource savings
- Can attach external nodes (GPU, EC2)
- Built-in Web UI
- Team sharing via VPN (Tailscale)

**Cons:**
- Relatively new project, ecosystem less mature than Kind
- Cannot directly reuse existing Kind test scripts
- Still requires Docker underneath — does not solve the DinD problem itself

**Recommendation:** Worth evaluating as a Kind replacement in specific scenarios, but not a solution to the DinD problem.

## Summary Comparison

| Approach | Security | Performance | Kind Compat | Node Changes | Enterprise Ready |
|----------|----------|-------------|-------------|-------------|-----------------|
| DinD privileged | Low | Medium (vfs) | Full | No | Single-tenant only |
| **Sysbox** | **Medium-High** | **Near-native** | **Official** | **Install runtime** | **Medium** |
| **Rootless Podman** | **High** | **Near-native** | **Partial** | **No** | **RHEL/OpenShift** |
| Envbox | Medium-High | Near-native | Full | No | Coder-coupled |
| Kata/MicroVM | Highest | Medium (VM) | Theoretical | Bare metal | Large scale |
| vind | Medium | Near-native | Alternative | No | New project |

## Decision

Adopt a **phased strategy** with multiple supported approaches:

### Short-term: Privileged DinD

For controlled dev/test environments where security risk is acceptable. Quickest path to enabling DinD workloads. Document as "development-only, not for production."

### Medium-term: Sysbox (recommended default) + Rootless Podman (RHEL/OpenShift)

- **Sysbox** as the primary recommended runtime — best balance of security, performance, and Kind compatibility
- **Rootless Podman** as the recommended alternative for Red Hat / OpenShift ecosystems where customers require RHEL-aligned tooling or cannot install custom runtimes

### Long-term: Hardware Isolation

For enterprise multi-tenant scenarios, evaluate Kata Containers / MicroVM or a self-contained approach inspired by Coder Envbox.

### Impact on KubeOpenCode

- **No core code changes required** — all approaches are cluster-level infrastructure concerns
- KubeOpenCode's Agent `podSpec` already supports `runtimeClassName` passthrough
- A dedicated documentation page will guide cluster administrators on configuring their cluster for DinD workloads
- E2E testing documentation will reference Sysbox as the primary recommended approach

## Consequences

### Positive

- Users get a clear, documented path to enable DinD workloads
- Multiple options accommodate different enterprise environments (Docker-native, RHEL/OpenShift, high-security)
- No KubeOpenCode code changes needed
- Users who don't need DinD are completely unaffected

### Negative

- Cluster administrators must take action to enable DinD support
- Multiple supported approaches increase documentation burden
- Sysbox/Podman Kind compatibility may have edge cases requiring troubleshooting

## References

- [Sysbox GitHub](https://github.com/nestybox/sysbox)
- [Kind inside Sysbox Container](https://blog.nestybox.com/2022/01/10/kind-in-sysbox.html)
- [Sysbox Quick Start: Kind](https://github.com/nestybox/sysbox/blob/master/docs/quickstart/kind.md)
- [Kindbox — Kind clusters in Sysbox containers](https://github.com/nestybox/kindbox)
- [Podman Inside Container (Red Hat)](https://www.redhat.com/en/blog/podman-inside-container)
- [Podman Rootless Overlay (Red Hat)](https://www.redhat.com/en/blog/podman-rootless-overlay)
- [Kind Rootless / Podman Documentation](https://kind.sigs.k8s.io/docs/user/rootless/)
- [CERN: Rootless Container Builds on Kubernetes](https://kubernetes.web.cern.ch/blog/2025/06/19/rootless-container-builds-on-kubernetes/)
- [Coder Envbox](https://github.com/coder/envbox)
- [Kata Containers](https://katacontainers.io/)
- [Firecracker MicroVM](https://firecracker-microvm.github.io/)
- [vind — Kind Alternative](https://github.com/loft-sh/vind)
- [bsycorp/kind for CI](https://github.com/bsycorp/kind)
