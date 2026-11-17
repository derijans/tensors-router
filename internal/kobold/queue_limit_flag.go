package kobold

import (
	"bytes"
	"strconv"
	"sync"
)

const (
	queueLimitFlag               = "--queuelimit"
	legacyQueueLimitFlag         = "--multiuser"
	argparseRejectionMarker      = "unrecognized arguments:"
	rejectionScanWindowBytes     = 4096
	queueLimitFlagMinimumVersion = "KoboldCpp v1.123.1"
)

type unsupportedQueueLimitFlagError struct {
	err error
}

func (unsupported *unsupportedQueueLimitFlagError) Error() string {
	return unsupported.err.Error()
}

func (unsupported *unsupportedQueueLimitFlagError) Unwrap() error {
	return unsupported.err
}

func (manager *Manager) queueLimitArguments() []string {
	manager.state.Lock()
	flag := queueLimitFlag
	if manager.queueLimitFlagUnsupported {
		flag = legacyQueueLimitFlag
	}
	manager.state.Unlock()
	return []string{flag, strconv.Itoa(manager.config.Multiuser)}
}

func (manager *Manager) switchToLegacyQueueLimitFlag() bool {
	manager.state.Lock()
	defer manager.state.Unlock()
	if manager.queueLimitFlagUnsupported {
		return false
	}
	manager.queueLimitFlagUnsupported = true
	return true
}

type queueLimitFlagRejectionDetector struct {
	mu       sync.Mutex
	tail     []byte
	rejected bool
}

func (detector *queueLimitFlagRejectionDetector) Write(content []byte) (int, error) {
	detector.mu.Lock()
	defer detector.mu.Unlock()
	if detector.rejected {
		return len(content), nil
	}
	detector.tail = append(detector.tail, content...)
	detector.rejected = outputRejectsQueueLimitFlag(detector.tail)
	if len(detector.tail) > rejectionScanWindowBytes {
		detector.tail = append([]byte(nil), detector.tail[len(detector.tail)-rejectionScanWindowBytes:]...)
	}
	return len(content), nil
}

func (detector *queueLimitFlagRejectionDetector) Rejected() bool {
	detector.mu.Lock()
	defer detector.mu.Unlock()
	return detector.rejected
}

func outputRejectsQueueLimitFlag(output []byte) bool {
	for line := range bytes.Lines(output) {
		_, rejectedArguments, found := bytes.Cut(line, []byte(argparseRejectionMarker))
		if !found {
			continue
		}
		for _, argument := range bytes.Fields(rejectedArguments) {
			if string(argument) == queueLimitFlag {
				return true
			}
		}
	}
	return false
}
