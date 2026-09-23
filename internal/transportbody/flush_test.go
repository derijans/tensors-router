package transportbody

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

type flushCountingRecorder struct {
	*httptest.ResponseRecorder
	flushedBodies []string
}

func (recorder *flushCountingRecorder) Flush() {
	recorder.flushedBodies = append(recorder.flushedBodies, recorder.Body.String())
}

func TestCopyFlushingFlushesEachChunkAsItArrives(t *testing.T) {
	recorder := &flushCountingRecorder{ResponseRecorder: httptest.NewRecorder()}
	written, err := CopyFlushing(recorder, singleByteReader{reader: strings.NewReader("abc")})
	if err != nil || written != 3 {
		t.Fatalf("written=%d error=%v", written, err)
	}
	if got := strings.Join(recorder.flushedBodies, ","); got != "a,ab,abc" {
		t.Fatalf("flushed bodies = %q, want each chunk flushed on arrival", got)
	}
}

func TestCopyResponseFlushingStopsAtCap(t *testing.T) {
	recorder := httptest.NewRecorder()
	written, err := CopyResponseFlushing(recorder, strings.NewReader("12345"), 4)
	if !errors.Is(err, ErrResponseTooLarge) || written != 4 || recorder.Body.String() != "1234" {
		t.Fatalf("unexpected capped copy written=%d body=%q error=%v", written, recorder.Body.String(), err)
	}
}
