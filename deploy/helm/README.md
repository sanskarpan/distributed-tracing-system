# Helm Chart

The packaged Kubernetes install surface lives in [`tracing/`](./tracing).

Validate it locally:

```bash
helm lint deploy/helm/tracing
helm template tracing deploy/helm/tracing > /tmp/tracing-chart.yaml
```

Key values:

- `collector.image.*`
- `collector.env.authTokens`
- `collector.env.replicaPeers`
- `collector.env.alertWebhookUrl`
- `collector.persistence.*`
- `web.image.*`
- `ingress.*`
