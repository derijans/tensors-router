package transportbody

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestJSONTransformerForwardsKeepaliveWhitespaceBeforeBodyArrives(t *testing.T) {
	source, backend := io.Pipe()
	t.Cleanup(func() { _ = backend.CloseWithError(errors.New("test finished")) })
	transformed := NewJSONTransformReadCloser(source, JSONRewrite{
		Replacements: map[string]StringReplacement{PathModel: {To: "public"}},
	})
	defer transformed.Close()

	keepalive := strings.Repeat(" ", 2047) + "\n"
	go func() { _, _ = io.WriteString(backend, keepalive) }()
	forwarded := make(chan string, 1)
	go func() {
		buffer := make([]byte, len(keepalive))
		read, _ := io.ReadFull(transformed, buffer)
		forwarded <- string(buffer[:read])
	}()
	select {
	case got := <-forwarded:
		if got != keepalive {
			t.Fatalf("forwarded keepalive = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("keepalive whitespace was held until the JSON body arrived")
	}

	go func() {
		_, _ = io.WriteString(backend, `{"model":"local","images":["abc"]}`)
		_ = backend.Close()
	}()
	rest, err := io.ReadAll(transformed)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != `{"model":"public","images":["abc"]}` {
		t.Fatalf("transformed body = %s", rest)
	}
}
