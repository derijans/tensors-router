package downloader

import "context"

type Service interface {
	Capability() Capability
	Search(context.Context, SearchRequest, string) ([]SearchResult, error)
	SearchPage(context.Context, SearchRequest, string) (SearchPage, error)
	Repository(context.Context, RepositoryRequest) (RepositoryDetails, error)
	Plan(context.Context, PlanRequest) (DownloadPlan, error)
	CreateJob(context.Context, CreateJobRequest) (DownloadJob, error)
	Job(string) (DownloadJob, bool, error)
	Jobs() ([]DownloadJob, error)
	Artifacts() ([]ArtifactRecord, error)
	Pause(string) (DownloadJob, error)
	Resume(string) (DownloadJob, error)
	Cancel(string) (DownloadJob, error)
	Subscribe(string) (<-chan DownloadJob, func())
	Rescan() ([]ArtifactRecord, error)
	SetArtifactHandler(ArtifactHandler)
	Close() error
}

var _ Service = (*Manager)(nil)
