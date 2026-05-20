# Release Guide

## Artifact Types

- Collector binaries are built through [`.goreleaser.yml`](../.goreleaser.yml) for `linux/darwin` × `amd64/arm64`.
- Collector Docker image is published to `ghcr.io/<org>/tracing-collector:<version>` (multi-arch manifest).
- Web Docker image is published to `ghcr.io/<org>/tracing-web:<version>` (multi-arch manifest).
- The frontend release bundle is packaged as `tracing-web-dist.tar.gz`.
- The Kubernetes install surface is packaged as a Helm chart from [`deploy/helm/tracing`](../deploy/helm/tracing).

## Workflow

CI release automation is defined in [`.github/workflows/release.yml`](../.github/workflows/release.yml).

Tagging `v*` triggers:
1. Helm chart lint + template render validation
2. GoReleaser config sanity check
3. Explicit GitHub Actions `production-release` environment approval gate before publication
4. Collector multi-platform binary builds + GitHub release publication
5. Collector and web multi-arch Docker image build + push to GHCR
6. Stable `latest` tags are promoted only after versioned images have been published
7. Web bundle (`tracing-web-dist.tar.gz`) + Helm chart (`.tgz`) attached to the GitHub release

Manual `workflow_dispatch` runs are validation-only and do not publish release artifacts.

## Installing with Helm

Use release tags with the `v` prefix, for example `v1.0.0`.

```bash
# Download the chart from the GitHub release
helm install tracing tracing-v1.0.0.tgz \
  --set collector.image.tag=v1.0.0 \
  --set web.image.tag=v1.0.0
```

Override the image org if you've pushed to your own registry:

```bash
helm install tracing tracing-v1.0.0.tgz \
  --set collector.image.repository=ghcr.io/<your-org>/tracing-collector \
  --set collector.image.tag=v1.0.0 \
  --set web.image.repository=ghcr.io/<your-org>/tracing-web \
  --set web.image.tag=v1.0.0
```

## Local Validation

```bash
make helm-lint
make integration
```

For a local release dry run (skips publish and Docker push):

```bash
make release-dry-run
```
