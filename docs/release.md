# Release Guide

## Artifact Types

- Collector binaries are built through [`.goreleaser.yml`](../.goreleaser.yml).
- The frontend release bundle is packaged as `tracing-web-dist.tar.gz`.
- The Kubernetes install surface is packaged as a Helm chart from [`deploy/helm/tracing`](../deploy/helm/tracing).

## Workflow

- CI release automation is defined in [`.github/workflows/release.yml`](../.github/workflows/release.yml).
- Tagging `v*` triggers:
  - collector multi-platform binary builds
  - web bundle packaging
  - Helm chart packaging
  - GitHub release publication

## Local Validation

```bash
make helm-lint
make integration
```

For a local release dry run:

```bash
make release-dry-run
```
