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

func readReplayPrefix(source io.Reader, contentLength int64, limit int64, budget *Budget) ([][]byte, int64, []*Reservation, bool, error) {
	chunks := [][]byte{}
	reservations := []*Reservation{}
	remaining := limit
	var size int64
	for remaining > 0 {
		chunkSize := replayChunkBytes
		if chunkSize > remaining {
			chunkSize = remaining
		}
		if contentLength >= 0 {
			expectedRemaining := contentLength - size
			if expectedRemaining >= 0 && expectedRemaining < chunkSize {
				chunkSize = expectedRemaining + 1
			}
		}
		if chunkSize <= 0 {
			chunkSize = 1
		}
		if available := budget.Available(); chunkSize > available {
			chunkSize = available
		}
		if chunkSize <= 0 {
			var probe [1]byte
			read, err := io.ReadFull(source, probe[:])
			if read > 0 {
				return chunks, size, reservations, false, ErrBufferCapacity
			}
			if err == io.EOF {
				return chunks, size, reservations, true, nil
			}
			return chunks, size, reservations, false, err
		}
		reservation, ok := budget.Reserve(chunkSize)
		if !ok {
			return chunks, size, reservations, false, ErrBufferCapacity
		}
		reservations = append(reservations, reservation)
		chunk := make([]byte, int(chunkSize))
		read, err := io.ReadFull(source, chunk)
		if int64(read) < chunkSize {
			retained := append([]byte{}, chunk[:read]...)
			chunk = retained
			reservation.ShrinkTo(int64(read))
		}
		if read > 0 {
			chunks = append(chunks, chunk[:read])
			size += int64(read)
			remaining -= int64(read)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return chunks, size, reservations, true, nil
		}
		if err != nil {
			return chunks, size, reservations, false, err
		}
	}
	var probe [1]byte
	read, err := source.Read(probe[:])
	if read > 0 {
		reservation, ok := budget.Reserve(1)
		if !ok {
			return chunks, size, reservations, false, ErrBufferCapacity
		}
		reservations = append(reservations, reservation)
		chunks = append(chunks, []byte{probe[0]})
		size++
		return chunks, size, reservations, false, nil
	}
	if err == io.EOF {
		return chunks, size, reservations, true, nil
	}
	if err != nil {
		return chunks, size, reservations, false, err
	}
	return chunks, size, reservations, true, nil
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
