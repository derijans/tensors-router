package analytics

import (
	"strings"
	"unicode/utf8"
)

const maxErrorMessageBytes = 512

func clampErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= maxErrorMessageBytes {
		return message
	}
	cut := maxErrorMessageBytes
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut]
}

func queueWaitMS(event Event) int64 {
	if event.QueueWaitMS > 0 {
		return event.QueueWaitMS
	}
	if event.WorkStartedAt.IsZero() || !event.WorkStartedAt.After(event.StartedAt) {
		return 0
	}
	return event.WorkStartedAt.Sub(event.StartedAt).Milliseconds()
}
