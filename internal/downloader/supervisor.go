package downloader

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	restartInitialDelay = time.Second
	restartMaximumDelay = 30 * time.Second
	stableCompanionAge  = 30 * time.Second
)

type clientStarter func(context.Context) (*Client, error)

type SupervisedClient struct {
	start     clientStarter
	lifecycle context.Context
	stop      context.CancelFunc
	mu        sync.Mutex
	current   *Client
	startedAt time.Time
	outage    error
	handler   ArtifactHandler
	watcher   sync.WaitGroup
}

var _ Service = (*SupervisedClient)(nil)

func StartSupervisedClient(ctx context.Context, binaryPath string, configPath string) (*SupervisedClient, error) {
	return superviseClient(ctx, func(ctx context.Context) (*Client, error) {
		return StartClient(ctx, binaryPath, configPath)
	})
}

func superviseClient(ctx context.Context, start clientStarter) (*SupervisedClient, error) {
	client, err := start(ctx)
	if err != nil {
		return nil, err
	}
	lifecycle, stop := context.WithCancel(context.Background())
	supervisor := &SupervisedClient{start: start, lifecycle: lifecycle, stop: stop, current: client, startedAt: time.Now()}
	supervisor.watcher.Add(1)
	go supervisor.watch()
	return supervisor, nil
}

func (supervisor *SupervisedClient) watch() {
	defer supervisor.watcher.Done()
	delay := restartInitialDelay
	for {
		supervisor.mu.Lock()
		client, startedAt := supervisor.current, supervisor.startedAt
		supervisor.mu.Unlock()
		select {
		case <-client.Done():
		case <-supervisor.lifecycle.Done():
			return
		}
		supervisor.recordOutage(client.connectionError())
		if time.Since(startedAt) >= stableCompanionAge {
			delay = restartInitialDelay
		}
		if !supervisor.restart(&delay) {
			return
		}
	}
}

func (supervisor *SupervisedClient) restart(delay *time.Duration) bool {
	for {
		if waitWithContext(supervisor.lifecycle, *delay) != nil {
			return false
		}
		*delay = min(*delay*2, restartMaximumDelay)
		replacement, err := supervisor.start(supervisor.lifecycle)
		if err != nil {
			supervisor.recordOutage(err)
			continue
		}
		if supervisor.lifecycle.Err() != nil {
			_ = replacement.Close()
			return false
		}
		supervisor.install(replacement)
		return true
	}
}

func (supervisor *SupervisedClient) install(client *Client) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	client.SetArtifactHandler(supervisor.handler)
	supervisor.current, supervisor.startedAt, supervisor.outage = client, time.Now(), nil
}

func (supervisor *SupervisedClient) recordOutage(err error) {
	supervisor.mu.Lock()
	supervisor.outage = err
	supervisor.mu.Unlock()
}

func (supervisor *SupervisedClient) active() (*Client, error) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	select {
	case <-supervisor.current.Done():
		reason := supervisor.outage
		if reason == nil {
			reason = supervisor.current.connectionError()
		}
		return nil, &Error{Code: ErrorServiceUnavailable, Message: fmt.Sprintf("downloader companion is restarting after: %v", reason)}
	default:
		return supervisor.current, nil
	}
}

func (supervisor *SupervisedClient) Capability() Capability {
	client, err := supervisor.active()
	if err != nil {
		supervisor.mu.Lock()
		capability := supervisor.current.capability
		supervisor.mu.Unlock()
		capability.Available, capability.Error, capability.Reason = false, err.Error(), err.Error()
		return capability
	}
	return client.Capability()
}

func (supervisor *SupervisedClient) Search(ctx context.Context, request SearchRequest, token string) ([]SearchResult, error) {
	client, err := supervisor.active()
	if err != nil {
		return nil, err
	}
	return client.Search(ctx, request, token)
}

func (supervisor *SupervisedClient) SearchPage(ctx context.Context, request SearchRequest, token string) (SearchPage, error) {
	client, err := supervisor.active()
	if err != nil {
		return SearchPage{}, err
	}
	return client.SearchPage(ctx, request, token)
}

func (supervisor *SupervisedClient) Repository(ctx context.Context, request RepositoryRequest) (RepositoryDetails, error) {
	client, err := supervisor.active()
	if err != nil {
		return RepositoryDetails{}, err
	}
	return client.Repository(ctx, request)
}

func (supervisor *SupervisedClient) Plan(ctx context.Context, request PlanRequest) (DownloadPlan, error) {
	client, err := supervisor.active()
	if err != nil {
		return DownloadPlan{}, err
	}
	return client.Plan(ctx, request)
}

func (supervisor *SupervisedClient) CreateJob(ctx context.Context, request CreateJobRequest) (DownloadJob, error) {
	client, err := supervisor.active()
	if err != nil {
		return DownloadJob{}, err
	}
	return client.CreateJob(ctx, request)
}

func (supervisor *SupervisedClient) Job(id string) (DownloadJob, bool, error) {
	client, err := supervisor.active()
	if err != nil {
		return DownloadJob{}, false, err
	}
	return client.Job(id)
}

func (supervisor *SupervisedClient) Jobs() ([]DownloadJob, error) {
	client, err := supervisor.active()
	if err != nil {
		return nil, err
	}
	return client.Jobs()
}

func (supervisor *SupervisedClient) Artifacts() ([]ArtifactRecord, error) {
	client, err := supervisor.active()
	if err != nil {
		return nil, err
	}
	return client.Artifacts()
}

func (supervisor *SupervisedClient) Pause(id string) (DownloadJob, error) {
	client, err := supervisor.active()
	if err != nil {
		return DownloadJob{}, err
	}
	return client.Pause(id)
}

func (supervisor *SupervisedClient) Resume(id string) (DownloadJob, error) {
	client, err := supervisor.active()
	if err != nil {
		return DownloadJob{}, err
	}
	return client.Resume(id)
}

func (supervisor *SupervisedClient) Cancel(id string) (DownloadJob, error) {
	client, err := supervisor.active()
	if err != nil {
		return DownloadJob{}, err
	}
	return client.Cancel(id)
}

func (supervisor *SupervisedClient) Rescan() ([]ArtifactRecord, error) {
	client, err := supervisor.active()
	if err != nil {
		return nil, err
	}
	return client.Rescan()
}

func (supervisor *SupervisedClient) SetArtifactHandler(handler ArtifactHandler) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	supervisor.handler = handler
	supervisor.current.SetArtifactHandler(handler)
}

func (supervisor *SupervisedClient) Close() error {
	supervisor.stop()
	supervisor.watcher.Wait()
	supervisor.mu.Lock()
	client := supervisor.current
	supervisor.mu.Unlock()
	return client.Close()
}
