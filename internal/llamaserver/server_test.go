package llamaserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	srv := New("/models/model.gguf", "/models/mmproj.gguf", 8001, 99, 8192, true, "llama-server", "q4_0", "q4_0")

	if srv.model != "/models/model.gguf" {
		t.Errorf("model path mismatch")
	}
	if srv.mmproj != "/models/mmproj.gguf" {
		t.Errorf("mmproj path mismatch")
	}
	if srv.port != 8001 {
		t.Errorf("port mismatch")
	}
}

func TestServerURL(t *testing.T) {
	srv := New("/models/model.gguf", "/models/mmproj.gguf", 8001, 99, 8192, true, "", "q4_0", "q4_0")
	if srv.URL() != "http://127.0.0.1:8001" {
		t.Errorf("expected http://127.0.0.1:8001, got %s", srv.URL())
	}
}

func TestHealthWaitWithMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	srv := &Server{
		port: 12345,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.waitForHealth(ctx, 10*time.Second)
	if err == nil {
		t.Error("expected timeout error for non-running server")
	}
}

func TestStopBeforeStart(t *testing.T) {
	srv := New("/models/model.gguf", "/models/mmproj.gguf", 8001, 99, 8192, true, "", "q4_0", "q4_0")
	if err := srv.Stop(); err != nil {
		t.Errorf("Stop should not error before start: %v", err)
	}
}
