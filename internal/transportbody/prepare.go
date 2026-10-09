package transportbody

import (
	"errors"
	"io"
)

const replayChunkBytes = int64(256 * 1024)

func Prepare(source io.ReadCloser, contentLength int64, hasExternalSelector bool, limits Limits, budget *Budget) (Body, error) {
	limits = limits.Normalized()
	if source == nil {
		source = io.NopCloser(&emptyReader{})
	}
	if budget == nil {
		budget = NewBudget(limits.MemoryBudgetBytes)
	}
	if contentLength > limits.MaxRequestBytes {
		_ = source.Close()
		return nil, ErrRequestTooLarge
	}
	externalThreshold := limits.ReplayBufferBytes
	if limits.SelectorScanBytes < externalThreshold {
		externalThreshold = limits.SelectorScanBytes
	}
	if contentLength >= 0 && contentLength > externalThreshold && !hasExternalSelector {
		_ = source.Close()
		return nil, ErrSelectorRequired
	}
	if contentLength > limits.ReplayBufferBytes {
		return newStreamingBody(nil, source, limits.MaxRequestBytes, contentLength, true, nil), nil
	}

	readLimit := limits.ReplayBufferBytes
	if !hasExternalSelector && externalThreshold < limits.ReplayBufferBytes {
		readLimit = externalThreshold
	}
	chunks, size, reservations, complete, err := readReplayPrefix(source, contentLength, readLimit, budget)
	if err != nil {
		_ = source.Close()
		releaseReservations(reservations)
		return nil, err
	}
	if complete {
		_ = source.Close()
		return newReplayBody(chunks, size, reservations), nil
	}
	if !hasExternalSelector {
		_ = source.Close()
		releaseReservations(reservations)
		return nil, ErrSelectorRequired
	}
	return newStreamingBody(chunks, source, limits.MaxRequestBytes, contentLength, contentLength >= 0, reservations), nil
}

func releaseReservations(reservations []*Reservation) {
	for _, reservation := range reservations {
		reservation.Release()
	}
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

func IsLimitError(err error) bool {
	return errors.Is(err, ErrRequestTooLarge) || errors.Is(err, ErrResponseTooLarge)
}
