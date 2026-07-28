export const hfFilterCatalogVersion = 1;

export interface HFFilterGroup {
  id: string;
  label: string;
  values: string[];
}

export const hfFilterCatalog: Record<string, HFFilterGroup[]> = {
  main: [
    {id: "formats", label: "Popular formats", values: ["gguf", "safetensors", "pytorch", "onnx", "transformers", "diffusers"]},
    {id: "tasks", label: "Popular tasks", values: ["text-generation", "image-text-to-text", "text-to-image", "automatic-speech-recognition", "sentence-similarity"]}
  ],
  tasks: [
    {id: "multimodal", label: "Multimodal", values: ["image-text-to-text", "visual-question-answering", "document-question-answering", "video-text-to-text", "any-to-any"]},
    {id: "vision", label: "Computer vision", values: ["image-classification", "object-detection", "image-segmentation", "depth-estimation", "text-to-image", "image-to-image", "image-to-video", "unconditional-image-generation"]},
    {id: "nlp", label: "Natural language processing", values: ["text-generation", "text2text-generation", "fill-mask", "token-classification", "text-classification", "question-answering", "summarization", "translation", "sentence-similarity", "feature-extraction"]},
    {id: "audio", label: "Audio", values: ["automatic-speech-recognition", "text-to-speech", "audio-classification", "audio-to-audio", "text-to-audio", "voice-activity-detection"]},
    {id: "tabular", label: "Tabular", values: ["tabular-classification", "tabular-regression", "time-series-forecasting"]},
    {id: "reinforcement", label: "Reinforcement learning", values: ["reinforcement-learning", "robotics"]}
  ],
  libraries: [
    {id: "core", label: "Libraries", values: ["library:transformers", "library:diffusers", "library:gguf", "library:safetensors", "library:pytorch", "library:tensorflow", "library:jax", "library:mlx", "library:keras", "library:sentence-transformers", "library:timm", "library:spacy"]},
    {id: "apps", label: "Compatible apps", values: ["app:llama.cpp", "app:transformers", "app:diffusers", "app:vllm", "app:text-generation-inference"]}
  ],
  languages: [
    {id: "languages", label: "Languages", values: ["language:en", "language:zh", "language:es", "language:hi", "language:ar", "language:pt", "language:bn", "language:ru", "language:ja", "language:pa", "language:de", "language:jv", "language:ko", "language:fr", "language:te", "language:mr", "language:tr", "language:ta", "language:vi", "language:ur", "language:it", "language:multilingual"]}
  ],
  licenses: [
    {id: "licenses", label: "Licenses", values: ["license:apache-2.0", "license:mit", "license:bsd-3-clause", "license:cc-by-4.0", "license:cc-by-sa-4.0", "license:cc-by-nc-4.0", "license:openrail", "license:bigscience-openrail-m", "license:llama2", "license:llama3", "license:gemma", "license:other"]}
  ],
  other: [
    {id: "formats", label: "Formats", values: ["gguf", "safetensors", "pytorch", "onnx", "tensorboard", "adapter-transformers"]},
    {id: "quantization", label: "Quantization", values: ["4-bit", "8-bit", "awq", "gptq", "quanto", "bitsandbytes", "fp8", "quantized"]},
    {id: "apps", label: "Compatible apps", values: ["app:llama.cpp", "app:transformers", "app:diffusers", "app:vllm", "app:text-generation-inference"]},
    {id: "inference", label: "Inference", values: ["inference:true", "provider:hf-inference", "provider:fal-ai", "provider:replicate", "provider:together", "provider:fireworks-ai"]},
    {id: "datasets", label: "Trained datasets", values: ["dataset:HuggingFaceFW/fineweb", "dataset:allenai/c4", "dataset:wikipedia", "dataset:common_voice"]}
  ]
};
