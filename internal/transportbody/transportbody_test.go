package transportbody

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"sync"
	"testing"
)

func TestPrepareTransitionsBetweenReplayableAndStreaming(t *testing.T) {
	limits := Limits{ReplayBufferBytes: 8, MemoryBudgetBytes: 32, MaxRequestBytes: 64, MaxResponseBytes: 64, SelectorScanBytes: 8}
	budget := NewBudget(limits.MemoryBudgetBytes)
	replayable, err := Prepare(io.NopCloser(strings.NewReader("12345678")), -1, false, limits, budget)
	if err != nil {
		t.Fatal(err)
	}
	if !replayable.Replayable() {
		t.Fatal("expected replayable body")
	}
	if budget.Used() == 0 {
		t.Fatal("replay buffer was not reserved")
	}
	if err := replayable.Close(); err != nil {
		t.Fatal(err)
	}
	if budget.Used() != 0 {
		t.Fatalf("replay reservation leaked: %d", budget.Used())
	}

	streaming, err := Prepare(io.NopCloser(strings.NewReader("123456789")), -1, true, limits, budget)
	if err != nil {
		t.Fatal(err)
	}
	if streaming.Replayable() {
		t.Fatal("expected streaming body")
	}
	attempt, err := streaming.OpenAttempt()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "123456789" {
		t.Fatalf("stream prefix was not preserved: %q", content)
	}
	_ = attempt.Close()
	if streaming.BytesConsumed() != int64(len(content)) {
		t.Fatalf("unexpected consumed byte count %d", streaming.BytesConsumed())
	}
	if streaming.CanRetry() {
		t.Fatal("consumed stream was retryable")
	}
	_ = streaming.Close()
}

func TestMultiGiBBodyStreamsWithoutReplayAllocation(t *testing.T) {
	const size = 3 * GiB
	limits := Limits{
		ReplayBufferBytes: 64 * MiB,
		MemoryBudgetBytes: 128 * MiB,
		MaxRequestBytes:   4 * GiB,
		MaxResponseBytes:  GiB,
		SelectorScanBytes: 64 * MiB,
	}
	budget := NewBudget(limits.MemoryBudgetBytes)
	body, err := Prepare(io.NopCloser(&syntheticReader{remaining: size}), size, true, limits, budget)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if body.Replayable() || budget.Used() != 0 {
		t.Fatalf("large stream retained replay memory replayable=%t used=%d", body.Replayable(), budget.Used())
	}
	attempt, err := body.OpenAttempt()
	if err != nil {
		t.Fatal(err)
	}
	destination := &countingWriter{}
	written, err := Copy(destination, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Close(); err != nil {
		t.Fatal(err)
	}
	if written != size || destination.written != size || body.BytesConsumed() != size {
		t.Fatalf("large stream was truncated written=%d destination=%d consumed=%d", written, destination.written, body.BytesConsumed())
	}
}

func TestPrepareRequiresEarlySelectorAndEnforcesCapacity(t *testing.T) {
	limits := Limits{ReplayBufferBytes: 8, MemoryBudgetBytes: 8, MaxRequestBytes: 64, MaxResponseBytes: 64, SelectorScanBytes: 4}
	_, err := Prepare(io.NopCloser(strings.NewReader("12345")), 5, false, limits, NewBudget(8))
	if !errors.Is(err, ErrSelectorRequired) {
		t.Fatalf("unexpected selector error %v", err)
	}
	_, err = Prepare(io.NopCloser(strings.NewReader("1234")), 4, false, limits, NewBudget(3))
	if !errors.Is(err, ErrBufferCapacity) {
		t.Fatalf("unexpected capacity error %v", err)
	}
	_, err = Prepare(io.NopCloser(strings.NewReader("123456789")), 65, true, limits, NewBudget(8))
	if !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("unexpected request cap error %v", err)
	}
}

func TestPrepareUnknownTinyBodyUsesOnlyRetainedBudget(t *testing.T) {
	limits := Limits{ReplayBufferBytes: 8, MemoryBudgetBytes: 1, MaxRequestBytes: 64, MaxResponseBytes: 64, SelectorScanBytes: 8}
	budget := NewBudget(1)
	body, err := Prepare(io.NopCloser(strings.NewReader("x")), -1, false, limits, budget)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Used() != 1 || !body.Replayable() {
		t.Fatalf("unexpected retained budget=%d replayable=%t", budget.Used(), body.Replayable())
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if budget.Used() != 0 {
		t.Fatalf("reservation leaked: %d", budget.Used())
	}
}

func TestBudgetRejectsConcurrentOvercommit(t *testing.T) {
	budget := NewBudget(64)
	start := make(chan struct{})
	release := make(chan struct{})
	results := make(chan bool, 16)
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			reservation, ok := budget.Reserve(16)
			if ok {
				defer reservation.Release()
			}
			results <- ok
			if ok {
				<-release
			}
		}()
	}
	close(start)
	succeeded := 0
	for range 16 {
		ok := <-results
		if ok {
			succeeded++
		}
	}
	close(release)
	wait.Wait()
	close(results)
	if succeeded == 0 || succeeded > 4 {
		t.Fatalf("unexpected concurrent reservations %d", succeeded)
	}
}

func TestJSONTransformerStreamsLargeBase64AcrossSingleByteReads(t *testing.T) {
	large := strings.Repeat("A", 2*1024*1024)
	source := `{"model":"public","media":"data:image/png;base64,` + large + `","override_settings":{"sd_model_checkpoint":"public-image"}}`
	reader := NewJSONTransformReadCloser(io.NopCloser(singleByteReader{reader: strings.NewReader(source)}), JSONRewrite{Replacements: map[string]StringReplacement{
		PathModel:              {From: "public", To: "local"},
		PathOverrideImageModel: {From: "public-image", To: "local-image"},
	}})
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte(`"model":"local"`)) || !bytes.Contains(result, []byte(`"sd_model_checkpoint":"local-image"`)) {
		t.Fatalf("selectors were not rewritten: %.200s", result)
	}
	if bytes.Count(result, []byte("A")) != len(large) {
		t.Fatal("large string was not copied exactly")
	}
}

func TestJSONTransformerRejectsInvalidEscapeOutsideSelector(t *testing.T) {
	reader := NewJSONTransformReadCloser(io.NopCloser(strings.NewReader(`{"model":"public","prompt":"bad\q"}`)), JSONRewrite{})
	if _, err := io.ReadAll(reader); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("unexpected invalid escape result %v", err)
	}
}

func TestJSONTransformerRejectsExcessiveNesting(t *testing.T) {
	depth := maxJSONNesting + 1
	source := `{"value":` + strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth) + "}"
	reader := NewJSONTransformReadCloser(io.NopCloser(strings.NewReader(source)), JSONRewrite{})
	if _, err := io.ReadAll(reader); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("unexpected nesting result %v", err)
	}
}

func TestMultipartTransformerStreamsFileAndRewritesModel(t *testing.T) {
	var source bytes.Buffer
	writer := multipart.NewWriter(&source)
	if err := writer.WriteField("model", "public"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{1, 2, 3, 4}, 512*1024)
	if _, err := file.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := Prepare(io.NopCloser(bytes.NewReader(source.Bytes())), int64(source.Len()), true, Limits{ReplayBufferBytes: 8, MemoryBudgetBytes: 16, MaxRequestBytes: int64(source.Len() + 1), MaxResponseBytes: 64, SelectorScanBytes: 8}, NewBudget(16))
	if err != nil {
		t.Fatal(err)
	}
	transformed, boundary, err := TransformMultipart(body, writer.Boundary(), MultipartRewrite{Fields: map[string]StringReplacement{"model": {From: "public", To: "local"}}})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := transformed.OpenAttempt()
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(attempt, boundary)
	modelPart, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	model, err := io.ReadAll(modelPart)
	if err != nil || string(model) != "local" {
		t.Fatalf("unexpected model %q error %v", model, err)
	}
	filePart, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	transferred, err := io.ReadAll(filePart)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transferred, payload) {
		t.Fatal("multipart file changed")
	}
	_ = attempt.Close()
	_ = transformed.Close()
}

func TestResponseCopyStopsAtCap(t *testing.T) {
	var destination bytes.Buffer
	written, err := CopyResponse(&destination, strings.NewReader("12345"), 4)
	if !errors.Is(err, ErrResponseTooLarge) || written != 4 || destination.String() != "1234" {
		t.Fatalf("unexpected capped copy written=%d body=%q error=%v", written, destination.String(), err)
	}
}

type singleByteReader struct {
	reader io.Reader
}

func (reader singleByteReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return reader.reader.Read(buffer)
}

type syntheticReader struct {
	remaining int64
}

func (reader *syntheticReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	read := int64(len(buffer))
	if read > reader.remaining {
		read = reader.remaining
	}
	reader.remaining -= read
	return int(read), nil
}

type countingWriter struct {
	written int64
}

func (writer *countingWriter) Write(buffer []byte) (int, error) {
	writer.written += int64(len(buffer))
	return len(buffer), nil
}
