# Whisper.cpp

The llama_sdcpp backend manages whisper-server as a third lazy native process for speech-to-text. It is independent from llama-server and sd-server, starts only for a transcription configuration, probes /health, writes whisper-server.log when backend disk logging is enabled, and stops during voice/all unloads, backend switches, and shutdown.

The compatibility baseline is [whisper.cpp v1.9.3](https://github.com/ggml-org/whisper.cpp/releases/tag/v1.9.3). Configure a direct archive or executable with updates.whispercpp_binary_url and its SHA-256 field, or use the default GitHub repository with an optional exact asset glob. Archives are normalized and installed with runtime libraries beside whisper-server.

## Router configuration

    whispercpp:
      backend_url: "http://127.0.0.1:5003"
      binary_path: "./bin/whisper/whisper-server"
      data_dir: "./data/whispercpp"
      hide_window: true
      extra_args: []

The router owns bind, port, public/request/inference paths, conversion, and temporary-directory behavior. Conflicting extra arguments are rejected. whisper-server itself still only accepts native RIFF/WAVE input; the router converts non-WAV uploads with ffmpeg on the buffered request path when `ffmpeg.binary_path` (or `PATH`) resolves a working binary, and rejects them explicitly otherwise. The streaming (large-body) request path still requires native WAV regardless of ffmpeg availability.

## KCPPS configuration

    {
      "backend_mode": "llama_sdcpp",
      "whispermodel": "C:/models/ggml-large-v3.bin",
      "threads": 8,
      "maingpu": 0,
      "flashattention": true,
      "whispercpp_language": "auto",
      "whispercpp_vad": true,
      "whispercpp_vad_model": "C:/models/ggml-silero-v6.2.0.bin"
    }

Shared fields map whispermodel to --model, threads to --threads, maingpu to --device, flashattention to the enabled/disabled flash-attention flag, and usecpu to --no-gpu. Native fields use whispercpp_* for processors, offsets, duration and segment/context limits, sampling thresholds, translation/diarization/fallback/output controls, language and prompt, OpenVINO/DTW, suppression, language probabilities, and all VAD controls. The VAD model participates in portable hashing, inventory, resolution, cooking, and constructor assignment.

## Public routes and examples

    curl http://127.0.0.1:8080/v1/audio/transcriptions -F file=@sample.wav -F model=whisper-large -F response_format=verbose_json

    curl http://127.0.0.1:8080/v1/audio/translations -F file=@sample.wav -F model=whisper-large -F response_format=srt

    curl http://127.0.0.1:8080/api/extra/transcribe -H "Content-Type: application/json" -d '{"model":"whisper-large","audio_data":"<base64-wav>","langcode":"lv","prompt":"Names"}'

OpenAI routes accept json, verbose_json, text, srt, and vtt. The router forces transcription or English translation by route and requests verbose backend data internally. Kobold input accepts audio_data, prompt, langcode or language, and suppress_non_speech, and returns the Kobold text shape.

The selector may instead use X-Tensors-Model or the model query parameter. With no selector, the master prefers loaded local STT, loaded healthy remote STT, a wholly idle capable node, its own compatible queue, and finally the shortest healthy remote queue. Whole-node counts come from the authenticated runtime-status endpoint. Older nodes remain available for explicit routing but are excluded from automatic selection.

Analytics retain duration, detected language, task, latency, status, transfer sizes, route, model, backend mode, and VRAM. Transcript, segment text, words, and language-probability maps are never stored.

The native UI is available at /router/webuis/whispercpp/ after enabling its WebUI session and loading a compatible model through router controls. Static UI, /health, and /inference are proxied. Direct upstream /load is denied.

## Troubleshooting

- A 400 WAV error means the upload is not RIFF/WAVE and either ffmpeg is unavailable or the request came in on the streaming (large-body) path, which does not convert.
- A readiness timeout means whisper-server did not answer /health; inspect whisper-server.log.
- An automatic-routing 503 means no healthy runtime-status-capable node advertised a whispermodel configuration.
- Change models through router load controls, never upstream /load.
