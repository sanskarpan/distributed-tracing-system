package demo

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// Scenario defines a named demo scenario with a weight for random selection.
type Scenario struct {
	Name   string
	Weight int
	Run    func(sdk *DemoSDK) error
}

// Scenarios is the weighted list of demo scenarios.
var Scenarios = []Scenario{
	{Name: "successful_checkout", Weight: 60, Run: runSuccessfulCheckout},
	{Name: "payment_timeout", Weight: 12, Run: runPaymentTimeout},
	{Name: "inventory_error", Weight: 10, Run: runInventoryError},
	{Name: "slow_database", Weight: 10, Run: runSlowDatabase},
	{Name: "retry_success", Weight: 5, Run: runRetrySuccess},
	{Name: "cache_miss_cascade", Weight: 3, Run: runCacheMissCascade},
}

func jitter(minMs, maxMs int) time.Duration {
	if maxMs <= minMs {
		return time.Duration(minMs) * time.Millisecond
	}
	return time.Duration(minMs+rand.Intn(maxMs-minMs)) * time.Millisecond
}

func runSuccessfulCheckout(sdk *DemoSDK) error {
	ctx := context.Background()

	ctx, frontendSpan := sdk.StartSpan(ctx, "HTTP GET /checkout", model.SpanKindServer, WithService("frontend-svc"))
	frontendSpan.Attributes = append(frontendSpan.Attributes,
		model.StringKV("http.method", "GET"),
		model.StringKV("http.url", "/checkout"),
		model.IntKV("http.status_code", 200),
	)
	time.Sleep(jitter(5, 15))

	gwCtx, gwSpan := sdk.StartSpan(ctx, "GET /api/checkout", model.SpanKindServer, WithService("api-gateway"))
	time.Sleep(jitter(2, 5))

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		invCtx, invClientSpan := sdk.StartSpan(gwCtx, "HTTP GET inventory", model.SpanKindClient, WithService("api-gateway"))
		invClientSpan.Attributes = append(invClientSpan.Attributes, model.StringKV("peer.service", "inventory-svc"))
		time.Sleep(jitter(2, 5))

		_, invSvcSpan := sdk.StartSpan(invCtx, "GET /api/inventory", model.SpanKindServer, WithService("inventory-svc"))

		_, cacheSpan := sdk.StartSpan(invCtx, "cache.get", model.SpanKindInternal, WithService("inventory-svc"))
		cacheSpan.Attributes = append(cacheSpan.Attributes,
			model.StringKV("db.system", "redis"),
			model.StringKV("cache.hit", "true"),
		)
		time.Sleep(jitter(1, 3))
		sdk.FinishSpan(cacheSpan)

		_, dbSpan := sdk.StartSpan(invCtx, "db.query", model.SpanKindInternal, WithService("inventory-svc"))
		dbSpan.Attributes = append(dbSpan.Attributes,
			model.StringKV("db.system", "postgresql"),
			model.StringKV("db.statement", "SELECT stock FROM inventory WHERE product_id = $1"),
		)
		time.Sleep(jitter(5, 15))
		sdk.FinishSpan(dbSpan)

		sdk.FinishSpan(invSvcSpan)
		sdk.FinishSpan(invClientSpan)
	}()

	go func() {
		defer wg.Done()
		payCtx, payClientSpan := sdk.StartSpan(gwCtx, "HTTP POST payment", model.SpanKindClient, WithService("api-gateway"))
		payClientSpan.Attributes = append(payClientSpan.Attributes, model.StringKV("peer.service", "payment-svc"))
		time.Sleep(jitter(2, 5))

		_, paySvcSpan := sdk.StartSpan(payCtx, "POST /api/charge", model.SpanKindServer, WithService("payment-svc"))

		_, redisSpan := sdk.StartSpan(payCtx, "redis.get", model.SpanKindInternal, WithService("payment-svc"))
		time.Sleep(jitter(2, 5))
		sdk.FinishSpan(redisSpan)

		_, stripeSpan := sdk.StartSpan(payCtx, "stripe.charge", model.SpanKindInternal, WithService("payment-svc"))
		stripeSpan.Attributes = append(stripeSpan.Attributes, model.StringKV("stripe.method", "card"))
		time.Sleep(jitter(50, 200))
		sdk.FinishSpan(stripeSpan)

		_, dbInsertSpan := sdk.StartSpan(payCtx, "db.insert", model.SpanKindInternal, WithService("payment-svc"))
		time.Sleep(jitter(5, 10))
		sdk.FinishSpan(dbInsertSpan)

		sdk.FinishSpan(paySvcSpan)
		sdk.FinishSpan(payClientSpan)
	}()

	wg.Wait()
	sdk.FinishSpan(gwSpan)
	sdk.FinishSpan(frontendSpan)

	return sdk.Export()
}

func runPaymentTimeout(sdk *DemoSDK) error {
	ctx := context.Background()
	ctx, root := sdk.StartSpan(ctx, "HTTP GET /checkout", model.SpanKindServer, WithService("frontend-svc"))
	time.Sleep(jitter(5, 10))

	gwCtx, gwSpan := sdk.StartSpan(ctx, "GET /api/checkout", model.SpanKindServer, WithService("api-gateway"))
	time.Sleep(jitter(2, 5))

	payCtx, payClientSpan := sdk.StartSpan(gwCtx, "HTTP POST payment", model.SpanKindClient, WithService("api-gateway"))
	payClientSpan.Attributes = append(payClientSpan.Attributes, model.StringKV("peer.service", "payment-svc"))

	_, paySvcSpan := sdk.StartSpan(payCtx, "POST /api/charge", model.SpanKindServer, WithService("payment-svc"))

	_, stripeSpan := sdk.StartSpan(payCtx, "stripe.charge", model.SpanKindInternal, WithService("payment-svc"))
	time.Sleep(3100 * time.Millisecond)
	sdk.SetError(stripeSpan, errors.New("payment_timeout: stripe took too long"))
	sdk.AddEvent(stripeSpan, "timeout", model.StringKV("timeout_ms", "3000"))
	sdk.FinishSpan(stripeSpan)

	sdk.SetError(paySvcSpan, errors.New("payment_timeout"))
	sdk.FinishSpan(paySvcSpan)
	sdk.SetError(payClientSpan, errors.New("payment_timeout"))
	sdk.FinishSpan(payClientSpan)
	sdk.SetError(gwSpan, errors.New("payment_timeout"))
	sdk.FinishSpan(gwSpan)
	sdk.FinishSpan(root)

	return sdk.Export()
}

func runInventoryError(sdk *DemoSDK) error {
	ctx := context.Background()
	ctx, root := sdk.StartSpan(ctx, "HTTP GET /checkout", model.SpanKindServer, WithService("frontend-svc"))
	time.Sleep(jitter(5, 10))

	gwCtx, gwSpan := sdk.StartSpan(ctx, "GET /api/checkout", model.SpanKindServer, WithService("api-gateway"))

	invCtx, invClientSpan := sdk.StartSpan(gwCtx, "HTTP GET inventory", model.SpanKindClient, WithService("api-gateway"))
	invClientSpan.Attributes = append(invClientSpan.Attributes, model.StringKV("peer.service", "inventory-svc"))

	_, invSvcSpan := sdk.StartSpan(invCtx, "GET /api/inventory", model.SpanKindServer, WithService("inventory-svc"))

	_, dbSpan := sdk.StartSpan(invCtx, "db.query", model.SpanKindInternal, WithService("inventory-svc"))
	time.Sleep(jitter(5, 15))
	sdk.SetError(dbSpan, errors.New("db error: connection refused"))
	sdk.FinishSpan(dbSpan)

	sdk.SetError(invSvcSpan, errors.New("inventory lookup failed"))
	sdk.FinishSpan(invSvcSpan)
	sdk.SetError(invClientSpan, errors.New("500 Internal Server Error"))
	sdk.FinishSpan(invClientSpan)
	sdk.SetError(gwSpan, errors.New("inventory error"))
	sdk.FinishSpan(gwSpan)
	sdk.FinishSpan(root)

	return sdk.Export()
}

func runSlowDatabase(sdk *DemoSDK) error {
	ctx := context.Background()
	ctx, root := sdk.StartSpan(ctx, "HTTP GET /checkout", model.SpanKindServer, WithService("frontend-svc"))
	time.Sleep(jitter(5, 10))

	gwCtx, gwSpan := sdk.StartSpan(ctx, "GET /api/checkout", model.SpanKindServer, WithService("api-gateway"))

	invCtx, invClientSpan := sdk.StartSpan(gwCtx, "HTTP GET inventory", model.SpanKindClient, WithService("api-gateway"))
	invClientSpan.Attributes = append(invClientSpan.Attributes, model.StringKV("peer.service", "inventory-svc"))

	_, invSvcSpan := sdk.StartSpan(invCtx, "GET /api/inventory", model.SpanKindServer, WithService("inventory-svc"))

	_, dbSpan := sdk.StartSpan(invCtx, "db.query", model.SpanKindInternal, WithService("inventory-svc"))
	dbSpan.Attributes = append(dbSpan.Attributes,
		model.StringKV("db.system", "postgresql"),
		model.StringKV("db.statement", "SELECT * FROM inventory -- full table scan"),
	)
	time.Sleep(800 * time.Millisecond)
	sdk.FinishSpan(dbSpan)

	sdk.FinishSpan(invSvcSpan)
	sdk.FinishSpan(invClientSpan)
	sdk.FinishSpan(gwSpan)
	sdk.FinishSpan(root)

	return sdk.Export()
}

func runRetrySuccess(sdk *DemoSDK) error {
	ctx := context.Background()
	ctx, root := sdk.StartSpan(ctx, "HTTP GET /checkout", model.SpanKindServer, WithService("frontend-svc"))
	time.Sleep(jitter(5, 10))

	gwCtx, gwSpan := sdk.StartSpan(ctx, "GET /api/checkout", model.SpanKindServer, WithService("api-gateway"))

	// First attempt: fail
	payCtx, payClient1 := sdk.StartSpan(gwCtx, "HTTP POST payment (attempt 1)", model.SpanKindClient, WithService("api-gateway"))
	_, paySvc1 := sdk.StartSpan(payCtx, "POST /api/charge", model.SpanKindServer, WithService("payment-svc"))
	time.Sleep(jitter(50, 100))
	sdk.SetError(paySvc1, errors.New("transient error"))
	sdk.FinishSpan(paySvc1)
	sdk.SetError(payClient1, errors.New("payment failed"))
	sdk.FinishSpan(payClient1)

	// Second attempt: success
	payCtx2, payClient2 := sdk.StartSpan(gwCtx, "HTTP POST payment (attempt 2)", model.SpanKindClient, WithService("api-gateway"))
	_, paySvc2 := sdk.StartSpan(payCtx2, "POST /api/charge", model.SpanKindServer, WithService("payment-svc"))
	time.Sleep(jitter(80, 150))
	sdk.FinishSpan(paySvc2)
	sdk.FinishSpan(payClient2)

	sdk.FinishSpan(gwSpan)
	sdk.FinishSpan(root)

	return sdk.Export()
}

func runCacheMissCascade(sdk *DemoSDK) error {
	ctx := context.Background()
	ctx, root := sdk.StartSpan(ctx, "HTTP GET /checkout", model.SpanKindServer, WithService("frontend-svc"))
	time.Sleep(jitter(5, 10))

	gwCtx, gwSpan := sdk.StartSpan(ctx, "GET /api/checkout", model.SpanKindServer, WithService("api-gateway"))

	invCtx, invClientSpan := sdk.StartSpan(gwCtx, "HTTP GET inventory", model.SpanKindClient, WithService("api-gateway"))
	invClientSpan.Attributes = append(invClientSpan.Attributes, model.StringKV("peer.service", "inventory-svc"))

	_, invSvcSpan := sdk.StartSpan(invCtx, "GET /api/inventory", model.SpanKindServer, WithService("inventory-svc"))

	_, cacheSpan := sdk.StartSpan(invCtx, "cache.get", model.SpanKindInternal, WithService("inventory-svc"))
	cacheSpan.Attributes = append(cacheSpan.Attributes,
		model.StringKV("db.system", "redis"),
		model.StringKV("cache.hit", "false"),
	)
	time.Sleep(jitter(1, 3))
	sdk.FinishSpan(cacheSpan)

	_, dbSpan := sdk.StartSpan(invCtx, "db.query", model.SpanKindInternal, WithService("inventory-svc"))
	dbSpan.Attributes = append(dbSpan.Attributes, model.StringKV("db.system", "postgresql"))
	time.Sleep(400 * time.Millisecond)
	sdk.FinishSpan(dbSpan)

	sdk.FinishSpan(invSvcSpan)
	sdk.FinishSpan(invClientSpan)
	sdk.FinishSpan(gwSpan)
	sdk.FinishSpan(root)

	return sdk.Export()
}
