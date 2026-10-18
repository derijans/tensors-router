package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const eventStreamDone = "[DONE]"

type eventStreamTranslator struct {
	source       io.ReadCloser
	lines        *bufio.Reader
	request      OpenAIRequest
	pending      bytes.Buffer
	toolCalls    generationText
	finishReason string
	usage        *openAIUsage
	finished     bool
}

func newEventStreamTranslator(source io.ReadCloser, request OpenAIRequest) *eventStreamTranslator {
	return &eventStreamTranslator{source: source, lines: bufio.NewReader(source), request: request}
}

func (stream *eventStreamTranslator) Read(buffer []byte) (int, error) {
	for stream.pending.Len() == 0 {
		if stream.finished {
			return 0, io.EOF
		}
		if err := stream.translateNextLine(); err != nil {
			return 0, err
		}
	}
	return stream.pending.Read(buffer)
}

func (stream *eventStreamTranslator) Close() error {
	return stream.source.Close()
}

func (stream *eventStreamTranslator) translateNextLine() error {
	line, err := stream.lines.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
		return stream.finish()
	}
	payload, isData := strings.CutPrefix(strings.TrimSpace(line), "data:")
	if !isData {
		return nil
	}
	payload = strings.TrimSpace(payload)
	if payload == eventStreamDone {
		return stream.finish()
	}
	var chunk openAIGeneration
	if json.Unmarshal([]byte(payload), &chunk) != nil {
		return nil
	}
	if len(chunk.Error) > 0 {
		stream.finished = true
		return stream.writeRecord(map[string]any{"error": errorMessage([]byte(payload), "backend stream failed")})
	}
	return stream.translateChunk(chunk)
}

func (stream *eventStreamTranslator) translateChunk(chunk openAIGeneration) error {
	if chunk.Usage != nil {
		stream.usage = chunk.Usage
	}
	if len(chunk.Choices) == 0 {
		return nil
	}
	choice := chunk.Choices[0]
	if choice.FinishReason != "" {
		stream.finishReason = choice.FinishReason
	}
	stream.toolCalls.add(openAIMessage{ToolCalls: choice.Delta.ToolCalls}, "")
	content := choice.Delta.Content + choice.Text
	if content == "" && choice.Delta.ReasoningContent == "" {
		return nil
	}
	return stream.writeRecord(stream.request.partialRecord(content, choice.Delta.ReasoningContent))
}

func (stream *eventStreamTranslator) finish() error {
	if stream.finished {
		return nil
	}
	stream.finished = true
	return stream.writeRecord(stream.request.doneRecord(stream.toolCalls, stream.finishReason, stream.usage))
}

func (stream *eventStreamTranslator) writeRecord(record map[string]any) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	stream.pending.Write(encoded)
	stream.pending.WriteByte('\n')
	return nil
}
