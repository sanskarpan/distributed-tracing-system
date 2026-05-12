package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPServerUsesProductionDefaults(t *testing.T) {
	srv := newHTTPServer(":4318", http.NewServeMux())

	assert.Equal(t, 5*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 15*time.Second, srv.ReadTimeout)
	assert.Equal(t, 30*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)
	assert.Equal(t, 1<<20, srv.MaxHeaderBytes)
}

func TestNewHTTPServerHonorsEnvOverrides(t *testing.T) {
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("HTTP_READ_TIMEOUT", "7s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "9s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "13s")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "4096")

	srv := newHTTPServer(":4318", http.NewServeMux())

	assert.Equal(t, 2*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 7*time.Second, srv.ReadTimeout)
	assert.Equal(t, 9*time.Second, srv.WriteTimeout)
	assert.Equal(t, 13*time.Second, srv.IdleTimeout)
	assert.Equal(t, 4096, srv.MaxHeaderBytes)
}
