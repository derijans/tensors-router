package clusterfan

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	Limit   = 8
	Timeout = 5 * time.Second
)

type Result[T any] struct {
	Target string
	Value  T
	Err    error
}

func Nodes[T any](ctx context.Context, targets []string, task func(context.Context, string) (T, error)) []Result[T] {
	return NodesWithin(ctx, targets, Timeout, Limit, task)
}

func NodesWithin[T any](ctx context.Context, targets []string, timeout time.Duration, limit int, task func(context.Context, string) (T, error)) []Result[T] {
	targets = UniqueTargets(targets)
	results := make([]Result[T], len(targets))
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
				results[index] = Result[T]{Target: targets[index], Value: value, Err: err}
			}
		}()
	}
	workers.Wait()
	return results
}

func UniqueTargets(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
