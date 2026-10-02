package downloader

import (
	"sync"
	"time"
)

const (
	subscriptionPollInterval = 250 * time.Millisecond
	subscriptionOutageLimit  = time.Minute
)

func (supervisor *SupervisedClient) Subscribe(id string) (<-chan DownloadJob, func()) {
	events := make(chan DownloadJob, 8)
	stop := make(chan struct{})
	var stopOnce sync.Once
	go supervisor.pollJob(id, events, stop)
	return events, func() { stopOnce.Do(func() { close(stop) }) }
}

func (supervisor *SupervisedClient) pollJob(id string, events chan<- DownloadJob, stop <-chan struct{}) {
	defer close(events)
	ticker := time.NewTicker(subscriptionPollInterval)
	defer ticker.Stop()
	var lastUpdate, failingSince time.Time
	for {
		job, found, err := supervisor.Job(id)
		switch {
		case err != nil:
			if failingSince.IsZero() {
				failingSince = time.Now()
			} else if time.Since(failingSince) > subscriptionOutageLimit {
				return
			}
		case !found:
			return
		default:
			failingSince = time.Time{}
			if !job.UpdatedAt.Equal(lastUpdate) {
				select {
				case events <- job:
					lastUpdate = job.UpdatedAt
				case <-stop:
					return
				}
			}
			if job.State.Terminal() {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-stop:
			return
		case <-supervisor.lifecycle.Done():
			return
		}
	}
}
