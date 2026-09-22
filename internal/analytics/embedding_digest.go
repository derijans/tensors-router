package analytics

const embeddingDigestLimit = 64 << 10

type vectorArrayCounting int

const (
	countEachElementAsVector vectorArrayCounting = iota
	countWholeArrayAsOneVector
)

var vectorArrayKeys = map[string]vectorArrayCounting{
	"data":       countEachElementAsVector,
	"embeddings": countEachElementAsVector,
	"embedding":  countWholeArrayAsOneVector,
}

type embeddingResponseDigest struct {
	digest            []byte
	overflowed        bool
	vectors           int64
	depth             int
	inString          bool
	escaped           bool
	expectingKey      bool
	capturingKey      bool
	key               []byte
	elidedArrayDepth  int
	awaitingElement   bool
	elidedArrayCounts vectorArrayCounting
}

func (digest *embeddingResponseDigest) Write(chunk []byte) {
	for _, value := range chunk {
		digest.consume(value)
	}
}

func (digest *embeddingResponseDigest) apply(event *Event, contentType string) {
	if !digest.overflowed {
		ApplyResponse(event, contentType, digest.digest)
	}
	event.EmbeddingCount = digest.vectors
}

func (digest *embeddingResponseDigest) eliding() bool {
	return digest.elidedArrayDepth > 0
}

func (digest *embeddingResponseDigest) consume(value byte) {
	if digest.inString {
		digest.consumeStringByte(value)
		return
	}
	if digest.eliding() && digest.depth == digest.elidedArrayDepth {
		digest.countElementStart(value)
	}
	switch value {
	case '"':
		digest.inString = true
		digest.capturingKey = digest.depth == 1 && digest.expectingKey
		digest.key = digest.key[:0]
	case '{', '[':
		digest.open(value)
		return
	case '}', ']':
		digest.close(value)
		return
	case ':':
		if digest.depth == 1 {
			digest.expectingKey = false
		}
	case ',':
		if digest.depth == 1 {
			digest.expectingKey = true
		}
	}
	digest.keep(value)
}

func (digest *embeddingResponseDigest) consumeStringByte(value byte) {
	digest.keep(value)
	switch {
	case digest.escaped:
		digest.escaped = false
	case value == '\\':
		digest.escaped = true
	case value == '"':
		digest.inString = false
		digest.capturingKey = false
		return
	}
	if digest.capturingKey {
		digest.key = append(digest.key, value)
	}
}

func (digest *embeddingResponseDigest) countElementStart(value byte) {
	switch value {
	case ' ', '\t', '\r', '\n', ']':
		return
	case ',':
		digest.awaitingElement = true
		return
	}
	if digest.awaitingElement && digest.elidedArrayCounts == countEachElementAsVector {
		digest.vectors++
	}
	digest.awaitingElement = false
}

func (digest *embeddingResponseDigest) open(value byte) {
	counting, isVectorArray := vectorArrayKeys[string(digest.key)]
	startsVectorArray := value == '[' && digest.depth == 1 && !digest.expectingKey && isVectorArray && !digest.eliding()
	digest.keep(value)
	digest.depth++
	if value == '{' && digest.depth == 1 {
		digest.expectingKey = true
	}
	if !startsVectorArray {
		return
	}
	digest.elidedArrayDepth = digest.depth
	digest.elidedArrayCounts = counting
	digest.awaitingElement = true
	if counting == countWholeArrayAsOneVector {
		digest.vectors++
	}
}

func (digest *embeddingResponseDigest) close(value byte) {
	if digest.eliding() && digest.depth == digest.elidedArrayDepth {
		digest.elidedArrayDepth = 0
	}
	digest.depth--
	digest.keep(value)
}

func (digest *embeddingResponseDigest) keep(value byte) {
	if digest.eliding() || digest.overflowed {
		return
	}
	if len(digest.digest) >= embeddingDigestLimit {
		digest.overflowed = true
		return
	}
	digest.digest = append(digest.digest, value)
}
