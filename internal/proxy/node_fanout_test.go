package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestFanOutNodesLimitsConcurrencyAndOrdersResults(t *testing.T) {
	targets := make([]string, 12)
	for index := range targets {
		targets[index] = fmt.Sprintf("node-%02d", len(targets)-index)
	}
	var active atomic.Int32
	var maximum atomic.Int32
	results := fanOutNodesWithin(context.Background(), targets, time.Second, 3, func(context.Context, string) (string, error) {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
		return "ok", nil
	})
	if maximum.Load() != 3 {
		t.Fatalf("expected concurrency 3, got %d", maximum.Load())
	}
	for index, result := range results {
		expected := fmt.Sprintf("node-%02d", index+1)
		if result.Target != expected || result.Value != "ok" || result.Err != nil {
			t.Fatalf("unexpected result %d: %#v", index, result)
		}
	}
}

func TestFanOutNodesUsesOneTimeoutBudget(t *testing.T) {
	started := time.Now()
	results := fanOutNodesWithin(context.Background(), []string{"a", "b", "c"}, 25*time.Millisecond, 2, func(ctx context.Context, target string) (string, error) {
		<-ctx.Done()
		return target, ctx.Err()
	})
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("fanout exceeded shared timeout: %s", elapsed)
	}
	for _, result := range results {
		if !errors.Is(result.Err, context.DeadlineExceeded) {
			t.Fatalf("unexpected timeout result %#v", result)
		}
	}
}
