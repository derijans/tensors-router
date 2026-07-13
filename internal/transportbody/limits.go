package transportbody

import "errors"

const (
	MiB                      = int64(1024 * 1024)
	GiB                      = int64(1024) * MiB
	TransformationWorkingSet = 32 * MiB
)

var (
	ErrBufferCapacity              = errors.New("transport memory budget capacity is unavailable")
	ErrInvalidJSON                 = errors.New("request body is not valid json")
	ErrInvalidChatTemplateKwargs   = errors.New("chat_template_kwargs must be an object or null")
	ErrDuplicateChatTemplateKwargs = errors.New("chat_template_kwargs must not be duplicated")
	ErrChatTemplateKwargsTooLarge  = errors.New("chat_template_kwargs exceeds the transformation limit")
	ErrRequestTooLarge             = errors.New("request body exceeds the configured streaming limit")
	ErrResponseTooLarge            = errors.New("response body exceeds the configured streaming limit")
	ErrSelectorRequired            = errors.New("a header or query model selector is required for streaming requests")
	ErrSelectorTooLarge            = errors.New("model selector exceeds 1 KiB")
	ErrStreamingBodyConsumed       = errors.New("streaming request body is not replayable after consumption")
	ErrStreamingBodyInUse          = errors.New("streaming request body already has an active attempt")
	ErrTransportBodyClosed         = errors.New("transport body is closed")
)

type Limits struct {
	ReplayBufferBytes int64
	MemoryBudgetBytes int64
	MaxRequestBytes   int64
	MaxResponseBytes  int64
	SelectorScanBytes int64
}

func DefaultLimits() Limits {
	return Limits{
		ReplayBufferBytes: 64 * MiB,
		MemoryBudgetBytes: 2 * GiB,
		MaxRequestBytes:   32 * GiB,
		MaxResponseBytes:  32 * GiB,
		SelectorScanBytes: 64 * MiB,
	}
}

func (limits Limits) Normalized() Limits {
	defaults := DefaultLimits()
	if limits.ReplayBufferBytes <= 0 {
		limits.ReplayBufferBytes = defaults.ReplayBufferBytes
	}
	if limits.MemoryBudgetBytes <= 0 {
		limits.MemoryBudgetBytes = defaults.MemoryBudgetBytes
	}
	if limits.MaxRequestBytes <= 0 {
		limits.MaxRequestBytes = defaults.MaxRequestBytes
	}
	if limits.MaxResponseBytes <= 0 {
		limits.MaxResponseBytes = defaults.MaxResponseBytes
	}
	if limits.SelectorScanBytes <= 0 {
		limits.SelectorScanBytes = defaults.SelectorScanBytes
	}
	if limits.ReplayBufferBytes > limits.MaxRequestBytes {
		limits.ReplayBufferBytes = limits.MaxRequestBytes
	}
	if limits.SelectorScanBytes > limits.MaxRequestBytes {
		limits.SelectorScanBytes = limits.MaxRequestBytes
	}
	return limits
}
