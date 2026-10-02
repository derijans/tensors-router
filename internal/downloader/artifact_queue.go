package downloader

import "sync"

type artifactQueue struct {
	mu      sync.Mutex
	pending []ArtifactRecord
	handler ArtifactHandler
	wake    chan struct{}
	stop    chan struct{}
	once    sync.Once
}

func newArtifactQueue() *artifactQueue {
	queue := &artifactQueue{wake: make(chan struct{}, 1), stop: make(chan struct{})}
	go queue.deliver()
	return queue
}

func (queue *artifactQueue) setHandler(handler ArtifactHandler) {
	queue.mu.Lock()
	queue.handler = handler
	queue.mu.Unlock()
	queue.signal()
}

func (queue *artifactQueue) push(record ArtifactRecord) {
	queue.mu.Lock()
	queue.pending = append(queue.pending, record)
	queue.mu.Unlock()
	queue.signal()
}

func (queue *artifactQueue) close() {
	queue.once.Do(func() { close(queue.stop) })
}

func (queue *artifactQueue) signal() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

func (queue *artifactQueue) deliver() {
	for {
		select {
		case <-queue.stop:
			return
		case <-queue.wake:
		}
		for {
			queue.mu.Lock()
			if len(queue.pending) == 0 || queue.handler == nil {
				queue.mu.Unlock()
				break
			}
			record, handler := queue.pending[0], queue.handler
			queue.pending = queue.pending[1:]
			queue.mu.Unlock()
			_ = handler(record)
		}
	}
}
