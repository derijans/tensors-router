package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
)

const openAIStreamReadSize = 32 * 1024

type openAIStreamUsageRequest struct {
	StreamOptions struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

type openAIUsageChunk struct {
	Choices []json.RawMessage `json:"choices"`
	Usage   koboldUsage       `json:"usage"`
}

func routerSuppliesKoboldOpenAIStreamUsage(path string, backendMode string, body []byte) bool {
	return backendMode == BackendModeKobold && requestStreamsOpenAIUsage(path) && streamUsageRequested(body)
}

func streamUsageRequested(body []byte) bool {
	var request openAIStreamUsageRequest
	return json.Unmarshal(body, &request) == nil && request.StreamOptions.IncludeUsage
}

type koboldOpenAIUsageSupplier struct {
	ctx               context.Context
	source            io.ReadCloser
	perf              koboldPerfReader
	generationsBefore int64
	partialLine       []byte
	ready             bytes.Buffer
	lastByteWritten   byte
	usageSettled      bool
	sourceErr         error
	readBuffer        []byte
}

func supplyKoboldOpenAIStreamUsage(ctx context.Context, source io.ReadCloser, perf koboldPerfReader, generationsBefore int64) io.ReadCloser {
	return &koboldOpenAIUsageSupplier{
		ctx:               ctx,
		source:            source,
		perf:              perf,
		generationsBefore: generationsBefore,
		readBuffer:        make([]byte, openAIStreamReadSize),
	}
}

func (supplier *koboldOpenAIUsageSupplier) Read(p []byte) (int, error) {
	for supplier.ready.Len() == 0 && supplier.sourceErr == nil {
		read, err := supplier.source.Read(supplier.readBuffer)
		if read > 0 {
			supplier.consume(supplier.readBuffer[:read])
		}
		if err == io.EOF {
			supplier.finishStream()
		}
		supplier.sourceErr = err
	}
	if supplier.ready.Len() > 0 {
		return supplier.ready.Read(p)
	}
	return 0, supplier.sourceErr
}

func (supplier *koboldOpenAIUsageSupplier) Close() error {
	return supplier.source.Close()
}

func (supplier *koboldOpenAIUsageSupplier) consume(chunk []byte) {
	supplier.partialLine = append(supplier.partialLine, chunk...)
	for {
		index := bytes.IndexByte(supplier.partialLine, '\n')
		if index < 0 {
			return
		}
		supplier.passLine(supplier.partialLine[:index+1])
		supplier.partialLine = supplier.partialLine[index+1:]
	}
}

func (supplier *koboldOpenAIUsageSupplier) finishStream() {
	if len(supplier.partialLine) > 0 {
		supplier.passLine(supplier.partialLine)
		supplier.partialLine = nil
	}
	supplier.supplyMissingUsage()
}

func (supplier *koboldOpenAIUsageSupplier) passLine(line []byte) {
	text := strings.TrimRight(string(line), "\r\n")
	switch {
	case isUsageOnlyEventLine(text):
		supplier.usageSettled = true
	case isStreamDoneEventLine(text):
		supplier.supplyMissingUsage()
	}
	supplier.write(line)
}

func (supplier *koboldOpenAIUsageSupplier) supplyMissingUsage() {
	if supplier.usageSettled {
		return
	}
	supplier.usageSettled = true
	usage, ok := supplier.perf.usageOfSingleGenerationSince(supplier.ctx, supplier.generationsBefore)
	if !ok {
		return
	}
	event := serverSentDataEvent(openAIUsageChunk{Choices: []json.RawMessage{}, Usage: usage})
	if event == nil {
		return
	}
	if supplier.lastByteWritten != 0 && supplier.lastByteWritten != '\n' {
		supplier.write([]byte{'\n'})
	}
	supplier.write(event)
}

func (supplier *koboldOpenAIUsageSupplier) write(data []byte) {
	if len(data) == 0 {
		return
	}
	_, _ = supplier.ready.Write(data)
	supplier.lastByteWritten = data[len(data)-1]
}

func isStreamDoneEventLine(line string) bool {
	payload, found := strings.CutPrefix(line, "data:")
	return found && strings.TrimSpace(payload) == "[DONE]"
}
