package vllm

import "context"

type Service interface {
	State(context.Context) State
	StartInitialization(context.Context, InitRequest) (InitializationJob, error)
	CancelInitialization(context.Context) (InitializationJob, error)
	Load(context.Context, RuntimeLoadRequest) (RuntimeStatus, error)
	Restart(context.Context, RuntimeKind) (RuntimeStatus, error)
	Unload(context.Context, RuntimeKind) error
	Runtime(context.Context, RuntimeKind) (RuntimeStatus, error)
	LaunchOptions(context.Context) (LaunchOptions, error)
	SetLaunchOptions(context.Context, LaunchOptions) (LaunchOptions, error)
	Close() error
}
