# Documentation

This repository now carries a dedicated `docs/` tree for system-level and contributor-facing documentation.

## Index

- [Architecture](./architecture.md)
  System boundaries, execution flow, major modules, and storage/runtime behavior.
- [API Guide](./api.md)
  Ingest, query, metrics, sampler, and SSE contract overview.
- [Frontend Guide](./frontend.md)
  UI structure, data flow, and page/component responsibilities.
- [Operations Guide](./operations.md)
  Configuration, deployment, observability, and shutdown behavior.
- [Development Guide](./development.md)
  Local workflows, testing, code layout, and contribution expectations.
- [Frontend Audit](./frontend-audit.md)
  Production-readiness gaps found during the frontend audit and how they were resolved.
- [Runbooks](./runbooks.md)
  Operational response playbooks for common collector and UI incidents.
- [Release Guide](./release.md)
  Artifact generation, packaging, and release automation.

## Audience

- New contributors should start with `development.md` and `architecture.md`.
- Operators should start with `operations.md`.
- On-call engineers should keep `runbooks.md` nearby.
- Frontend contributors should read `frontend.md` and `frontend-audit.md`.
- API consumers should use `api.md` plus the generated [`api/openapi.yaml`](../api/openapi.yaml).
