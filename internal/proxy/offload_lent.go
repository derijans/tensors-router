package proxy

import (
	"sort"
	"sync"
	"time"
)

type lentRequest struct {
	lane          string
	modelID       string
	arrived       time.Time
	helperNodeID  string
	helperModelID string
}

type lentRequestBook struct {
	mu      sync.Mutex
	byEntry map[*offloadEntry]lentRequest
}

func newLentRequestBook() *lentRequestBook {
	return &lentRequestBook{byEntry: map[*offloadEntry]lentRequest{}}
}

func (book *lentRequestBook) Add(entry *offloadEntry, request lentRequest) {
	book.mu.Lock()
	defer book.mu.Unlock()
	book.byEntry[entry] = request
}

func (book *lentRequestBook) Remove(entry *offloadEntry) (lentRequest, bool) {
	book.mu.Lock()
	defer book.mu.Unlock()
	request, lent := book.byEntry[entry]
	delete(book.byEntry, entry)
	return request, lent
}

func (book *lentRequestBook) Count(lane string, modelID string) int {
	book.mu.Lock()
	defer book.mu.Unlock()
	count := 0
	for _, request := range book.byEntry {
		if request.lane == lane && request.modelID == modelID {
			count++
		}
	}
	return count
}

func (book *lentRequestBook) Snapshot() []lentRequest {
	book.mu.Lock()
	requests := make([]lentRequest, 0, len(book.byEntry))
	for _, request := range book.byEntry {
		requests = append(requests, request)
	}
	book.mu.Unlock()
	sort.Slice(requests, func(left, right int) bool { return requests[left].arrived.Before(requests[right].arrived) })
	return requests
}
