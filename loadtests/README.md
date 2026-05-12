# Load Testing

This directory holds reusable load-generation assets for the collector.

## Prerequisites

- `k6` installed locally: <https://grafana.com/docs/k6/latest/set-up/install-k6/>
- collector running on `http://localhost:4318` or another reachable base URL

## Native Ingest Load

```bash
k6 run loadtests/k6/ingest-native-spans.js
```

Override the defaults with environment variables:

```bash
BASE_URL=http://localhost:4318 \
BATCH_SIZE=25 \
SERVICE_NAME=checkout \
k6 run loadtests/k6/ingest-native-spans.js
```

Supported environment variables:

- `BASE_URL`
  Collector base URL. Defaults to `http://localhost:4318`.
- `BATCH_SIZE`
  Number of spans per POST. Defaults to `20`.
- `SERVICE_NAME`
  Service name prefix used in generated spans. Defaults to `loadgen`.

## What It Exercises

- `/api/v1/spans` native ingest path
- sampler accept/drop pressure under concurrent write load
- assembler and store throughput with a steady stream of short traces

The script intentionally generates short parent/child traces so query, metrics, and sampler behavior all update during the run.
