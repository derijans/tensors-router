# Managed Separate Embedding Runtime

1. Extend `.kcpps`, catalog, Cook, validation, router configuration, examples, and documentation with `run_embed_separate` and dedicated loopback embedding endpoints.
2. Introduce independent KoboldCpp and llama.cpp embedding runtime state, role-specific launch construction, private KoboldCpp runtime configurations, and cleanup lifecycle.
3. Route embedding APIs lazily through the dedicated runtime while preserving current behavior by default and integrating unload, backend switching, health, logs, diagnostics, analytics, and VRAM measurements.
4. Add behavior-focused tests for parsing, generation, launch arguments, routing, lifecycle, failures, observability, and concurrency.
5. Run focused tests, race tests, the full Go suite, and all WebUI checks.
