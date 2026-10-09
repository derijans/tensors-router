package transportbody

import "io"

type replayPrefix struct {
	source        io.Reader
	contentLength int64
	budget        *Budget
	remaining     int64
	chunks        [][]byte
	reservations  []*Reservation
	size          int64
}

type replayStep struct {
	done     bool
	complete bool
	err      error
}

func readReplayPrefix(source io.Reader, contentLength int64, limit int64, budget *Budget) ([][]byte, int64, []*Reservation, bool, error) {
	prefix := replayPrefix{
		source:        source,
		contentLength: contentLength,
		budget:        budget,
		remaining:     limit,
		chunks:        [][]byte{},
		reservations:  []*Reservation{},
	}
	step := prefix.read()
	return prefix.chunks, prefix.size, prefix.reservations, step.complete, step.err
}

func (prefix *replayPrefix) read() replayStep {
	for prefix.remaining > 0 {
		chunkSize := prefix.nextChunkSize()
		var step replayStep
		if chunkSize <= 0 {
			step = prefix.probeWithoutBudget()
		} else {
			step = prefix.readChunk(chunkSize)
		}
		if step.done {
			return step
		}
	}
	return prefix.probeBeyondLimit()
}

func (prefix *replayPrefix) nextChunkSize() int64 {
	chunkSize := min(int64(replayChunkBytes), prefix.remaining)
	if prefix.contentLength >= 0 {
		expectedRemaining := prefix.contentLength - prefix.size
		if expectedRemaining >= 0 && expectedRemaining < chunkSize {
			chunkSize = expectedRemaining + 1
		}
	}
	if chunkSize <= 0 {
		chunkSize = 1
	}
	return min(chunkSize, prefix.budget.Available())
}

func (prefix *replayPrefix) probeWithoutBudget() replayStep {
	var probe [1]byte
	read, err := io.ReadFull(prefix.source, probe[:])
	if read > 0 {
		return replayStep{done: true, err: ErrBufferCapacity}
	}
	if err == io.EOF {
		return replayStep{done: true, complete: true}
	}
	return replayStep{done: true, err: err}
}

func (prefix *replayPrefix) readChunk(chunkSize int64) replayStep {
	reservation, ok := prefix.budget.Reserve(chunkSize)
	if !ok {
		return replayStep{done: true, err: ErrBufferCapacity}
	}
	prefix.reservations = append(prefix.reservations, reservation)
	chunk := make([]byte, int(chunkSize))
	read, err := io.ReadFull(prefix.source, chunk)
	if int64(read) < chunkSize {
		chunk = append([]byte{}, chunk[:read]...)
		reservation.ShrinkTo(int64(read))
	}
	if read > 0 {
		prefix.chunks = append(prefix.chunks, chunk[:read])
		prefix.size += int64(read)
		prefix.remaining -= int64(read)
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return replayStep{done: true, complete: true}
	}
	if err != nil {
		return replayStep{done: true, err: err}
	}
	return replayStep{}
}

func (prefix *replayPrefix) probeBeyondLimit() replayStep {
	var probe [1]byte
	read, err := prefix.source.Read(probe[:])
	if read > 0 {
		reservation, ok := prefix.budget.Reserve(1)
		if !ok {
			return replayStep{err: ErrBufferCapacity}
		}
		prefix.reservations = append(prefix.reservations, reservation)
		prefix.chunks = append(prefix.chunks, []byte{probe[0]})
		prefix.size++
		return replayStep{}
	}
	if err != nil && err != io.EOF {
		return replayStep{err: err}
	}
	return replayStep{complete: true}
}
