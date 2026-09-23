package proxy

import "sync"

type roundRobin struct {
	mu   sync.Mutex
	next uint64
}

func (rotation *roundRobin) pick(count int) int {
	if count == 0 {
		return 0
	}
	rotation.mu.Lock()
	defer rotation.mu.Unlock()
	index := int(rotation.next % uint64(count))
	rotation.next++
	return index
}
