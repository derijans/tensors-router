package proxy

const (
	headerContentType   = "Content-Type"
	headerContentLength = "Content-Length"
	headerTensorsModel  = "X-Tensors-Model"
	mediaTypeJSON       = "application/json"

	serverSentEventDataPrefix = "data: "
)

const (
	pathKoboldTranscribe    = "/api/extra/transcribe"
	pathKoboldVersion       = "/api/extra/version"
	pathAudioTranscriptions = "/v1/audio/transcriptions"
	pathAudioTranslations   = "/v1/audio/translations"
	pathOllamaGenerate      = "/api/generate"
	pathOllamaChat          = "/api/chat"
	pathEmbeddings          = "/v1/embeddings"
	pathEmbedV2             = "/v2/embed"
	pathRerank              = "/rerank"
	pathRerankV1            = "/v1/rerank"
	pathRerankV2            = "/v2/rerank"
	pathClassify            = "/classify"
	pathScore               = "/score"
	pathScoreV1             = "/v1/score"
	pathPooling             = "/pooling"
	pathSuffixEvents        = "/events"
	pathSuffixOutput        = "/output"
)

const (
	messageCaptureNotFound         = "capture not found"
	messageResolutionJobNotCreated = "resolution job could not be created"
	messageNodeIDMismatch          = "node_id does not match this node"
)
