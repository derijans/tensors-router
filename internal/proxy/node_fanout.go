package proxy

import (
	"context"
	"sync"
	"time"
)

const (
	nodeFanoutLimit   = 8
	nodeFanoutTimeout = 5 * time.Second
)

type nodeFanoutResult[T any] struct {
	Target string
	Value  T
	Err    error
}

func fanOutNodes[T any](ctx context.Context, targets []string, task func(context.Context, string) (T, error)) []nodeFanoutResult[T] {
	return fanOutNodesWithin(ctx, targets, nodeFanoutTimeout, nodeFanoutLimit, task)
}

func fanOutNodesWithin[T any](ctx context.Context, targets []string, timeout time.Duration, limit int, task func(context.Context, string) (T, error)) []nodeFanoutResult[T] {
	targets = uniqueSortedStrings(targets)
	results := make([]nodeFanoutResult[T], len(targets))
	if len(targets) == 0 {
		return results
	}
	if limit < 1 {
		limit = 1
	}
	if limit > len(targets) {
		limit = len(targets)
	}
	fanoutContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	jobs := make(chan int, len(targets))
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	var workers sync.WaitGroup
	workers.Add(limit)
	for range limit {
		go func() {
			defer workers.Done()
			for index := range jobs {
				value, err := task(fanoutContext, targets[index])
				results[index] = nodeFanoutResult[T]{Target: targets[index], Value: value, Err: err}
			}
		}()
	}
	workers.Wait()
	return results
}
