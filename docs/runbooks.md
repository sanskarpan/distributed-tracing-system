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

## `/readyz` Returns `503` With Status `overloaded`

1. Inspect the JSON body from `/readyz` and note `queueDepth`, `queueCapacity`, and `queueUsagePct`.
2. Confirm whether `READINESS_MAX_QUEUE_USAGE_PCT` is set intentionally for this environment.
3. Run `make loadtest-mixed` or `make soaktest` against staging to determine whether the threshold is too aggressive or the collector is genuinely saturated.
4. Check sampler configuration and trace volume. Reducing ingest pressure may be the fastest stabilizing move.
5. If sustained, capture `pprof` data in a trusted environment and look for blocked goroutines or hot paths.

## Unexpected Trace Drops

1. Compare `tracing_sampled_spans_total` versus `tracing_dropped_spans_total`.
2. Review the active sampler type and configuration.
3. If tail sampling is enabled, confirm buffer timeout and policy chain match the intended capture criteria.
4. Run `loadtests/k6/ingest-native-spans.js` against a staging collector to reproduce under controlled load.

## Memory or Goroutine Growth During Soak Runs

1. Enable `ENABLE_PPROF=true` in a trusted staging environment.
2. Capture `/debug/pprof/heap` and `/debug/pprof/goroutine`.
3. Run `make soaktest` and compare profiles before, during, and after the sustained window.
4. Correlate profile growth with `/readyz` queue fields and `/metrics` ingest pressure.

## Browser UI Looks Empty but Collector Is Healthy

1. Verify the web container can proxy `/api/`, `/sse/`, and `/v1/` to the collector.
2. Open `/api/v1/traces` directly from the browser network tab.
3. Confirm the SSE endpoints remain connected and are not being buffered by an upstream proxy.
4. If deploying behind ingress, make sure websocket/SSE buffering is disabled where required.
