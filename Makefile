.PHONY: build test race run demo dev lint loadtest loadtest-mixed soaktest

build:
	go build -o bin/collector ./cmd/collector
	go build -o bin/demo ./cmd/demo

test:
	go test ./... -v -count=1

race:
	go test ./... -race -count=1

run:
	go run ./cmd/collector &
	go run ./cmd/demo

dev:
	go run ./cmd/collector & go run ./cmd/demo & sleep 1 && (cd web && npm run dev)

demo:
	go run ./cmd/demo

lint:
	go vet ./...
	cd web && npx tsc --noEmit

loadtest:
	k6 run loadtests/k6/ingest-native-spans.js

loadtest-mixed:
	k6 run loadtests/k6/mixed-ingest-and-query.js

soaktest:
	k6 run loadtests/k6/collector-soak.js
