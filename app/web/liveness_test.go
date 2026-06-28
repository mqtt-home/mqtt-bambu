package web

import (
	"net/http"
	"testing"
	"time"
)

func TestEvaluateLiveness(t *testing.T) {
	grace := 4 * time.Minute
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("healthy returns 200 and clears timer", func(t *testing.T) {
		earlier := base.Add(-10 * time.Minute)
		code, since, stuck := evaluateLiveness(true, &earlier, base, grace)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if since != nil {
			t.Fatalf("expected cleared since, got %v", since)
		}
		if stuck != 0 {
			t.Fatalf("expected 0 stuck, got %v", stuck)
		}
	})

	t.Run("first unhealthy starts timer and stays 200", func(t *testing.T) {
		code, since, _ := evaluateLiveness(false, nil, base, grace)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if since == nil || !since.Equal(base) {
			t.Fatalf("expected since=base, got %v", since)
		}
	})

	t.Run("unhealthy within grace stays 200", func(t *testing.T) {
		start := base
		code, _, _ := evaluateLiveness(false, &start, base.Add(2*time.Minute), grace)
		if code != http.StatusOK {
			t.Fatalf("expected 200 within grace, got %d", code)
		}
	})

	t.Run("unhealthy beyond grace returns 503", func(t *testing.T) {
		start := base
		code, _, stuck := evaluateLiveness(false, &start, base.Add(5*time.Minute), grace)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 beyond grace, got %d", code)
		}
		if stuck != 5*time.Minute {
			t.Fatalf("expected 5m stuck, got %v", stuck)
		}
	})

	t.Run("recovery resets timer", func(t *testing.T) {
		start := base
		_, since, _ := evaluateLiveness(true, &start, base.Add(5*time.Minute), grace)
		if since != nil {
			t.Fatalf("expected reset timer on recovery, got %v", since)
		}
	})
}
