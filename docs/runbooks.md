# Runbooks

## Collector Starts but `/readyz` Returns `503`

Expected during graceful shutdown: the collector flips readiness to `draining` before it stops serving traffic.

If this happens unexpectedly:

1. Check the collector logs for shutdown signals or container restarts.
2. Inspect Kubernetes events or systemd logs for eviction, OOM, or deploy rollouts.
3. Confirm liveness still returns `200` on `/healthz`.
4. Verify upstream load balancers are honoring the readiness transition.

## Ingest Latency Spikes

1. Inspect `/metrics` for `tracing_latency_p99_ms` and `tracing_request_rate`.
2. Check sampler settings on `/api/v1/sampler` or the Sampler page.
3. If the collector is memory-backed, confirm trace volume has not reached retention pressure.
4. Use the Metrics and Timeline pages together to determine whether the spike is global or service-local.

## Unexpected Trace Drops

1. Compare `tracing_sampled_spans_total` versus `tracing_dropped_spans_total`.
2. Review the active sampler type and configuration.
3. If tail sampling is enabled, confirm buffer timeout and policy chain match the intended capture criteria.
4. Run `loadtests/k6/ingest-native-spans.js` against a staging collector to reproduce under controlled load.

## Browser UI Looks Empty but Collector Is Healthy

1. Verify the web container can proxy `/api/`, `/sse/`, and `/v1/` to the collector.
2. Open `/api/v1/traces` directly from the browser network tab.
3. Confirm the SSE endpoints remain connected and are not being buffered by an upstream proxy.
4. If deploying behind ingress, make sure websocket/SSE buffering is disabled where required.
