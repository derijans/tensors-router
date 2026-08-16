FROM node:24-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS web-assets
WORKDIR /source
COPY webui/package.json webui/package-lock.json ./webui/
RUN npm --prefix webui ci
COPY webui ./webui
COPY internal/webui/assets ./internal/webui/assets
RUN npm --prefix webui run build

FROM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS go-builder-musl
WORKDIR /source
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-assets /source/internal/webui/assets ./internal/webui/assets
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensors-router ./cmd/tensors-router
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensor-router-webui ./cmd/tensor-router-webui
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensor-router-downloader ./cmd/tensor-router-downloader

FROM ghcr.io/astral-sh/uv@sha256:606e70c71c852d03f611b1e56a195d08648507018a7057fab82c4974c4eae105 AS uv-bootstrap

FROM golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS go-builder-glibc
WORKDIR /source
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-assets /source/internal/webui/assets ./internal/webui/assets
COPY --from=uv-bootstrap /uv ./internal/vllm/assets/uv
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensors-router ./cmd/tensors-router
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensor-router-webui ./cmd/tensor-router-webui
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensor-router-downloader ./cmd/tensor-router-downloader
RUN /source/internal/vllm/assets/uv --version | awk '$1 == "uv" && $2 == "0.12.0" { found=1 } END { exit !found }'
RUN CGO_ENABLED=0 go build -tags vllm_embedded_uv -buildvcs=false -trimpath -ldflags "-s -w -X tensors-router/internal/buildinfo.Version=${VERSION} -X tensors-router/internal/buildinfo.Commit=${COMMIT} -X tensors-router/internal/buildinfo.Date=${BUILD_DATE}" -o /output/tensor-router-vllm ./cmd/tensor-router-vllm
RUN /output/tensor-router-vllm bootstrap-info | grep -E '^uv sha256:[0-9a-f]{64}$'

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce AS runtime-cpu
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 tensors && adduser -D -H -u 10001 -G tensors tensors && mkdir -p /config /models /data && chown -R tensors:tensors /data /models
WORKDIR /data
STOPSIGNAL SIGTERM

FROM debian:bookworm-slim@sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241 AS runtime-vllm
RUN apt-get update && apt-get install --yes --no-install-recommends ca-certificates libgomp1 tzdata && rm -rf /var/lib/apt/lists/*
RUN getent group video >/dev/null || groupadd --system video
RUN getent group render >/dev/null || groupadd --system render
RUN groupadd --gid 10001 tensors && useradd --no-create-home --uid 10001 --gid tensors tensors && install -d -o tensors -g tensors /config /models /data /data/vllm
WORKDIR /data
STOPSIGNAL SIGTERM

FROM nvidia/cuda:12.9.1-runtime-ubuntu24.04@sha256:1287141d283b8f06f45681b56a48a85791398c615888b1f96bfb9fc981392d98 AS runtime-cuda
RUN apt-get update && apt-get install --yes --no-install-recommends ca-certificates libgomp1 tzdata && rm -rf /var/lib/apt/lists/*
RUN getent group video >/dev/null || groupadd --system video
RUN getent group render >/dev/null || groupadd --system render
RUN groupadd --gid 10001 tensors && useradd --no-create-home --uid 10001 --gid tensors tensors && install -d -o tensors -g tensors /config /models /data /data/vllm
WORKDIR /data
STOPSIGNAL SIGTERM

FROM ubuntu:24.04@sha256:561618e2c15bf2397621dd04f96926663a3b5616c189cf7e38db7e82f5c538ea AS runtime-rocm
ARG ROCM_PACKAGE_VERSION=7.2.1.70201-81~24.04
ARG ROCM_INFO_PACKAGE_VERSION=1.0.0.70201-81~24.04
RUN apt-get update && apt-get install --yes --no-install-recommends ca-certificates curl gnupg libgomp1 tzdata && install -d -m 0755 /etc/apt/keyrings && curl --fail --location --silent --show-error https://repo.radeon.com/rocm/rocm.gpg.key -o /tmp/rocm.gpg.key && echo '2de99e2354646a90d9903e2a669fc4e36b02c1bbff7075c481e12d7edab2c88b  /tmp/rocm.gpg.key' | sha256sum -c - && gpg --batch --dearmor --output /etc/apt/keyrings/rocm.gpg /tmp/rocm.gpg.key && echo 'deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/rocm/apt/7.2.1 noble main' > /etc/apt/sources.list.d/rocm.list && apt-get update && apt-get install --yes --no-install-recommends rocm-hip-runtime="${ROCM_PACKAGE_VERSION}" rocm-hip-libraries="${ROCM_PACKAGE_VERSION}" rocminfo="${ROCM_INFO_PACKAGE_VERSION}" && apt-get purge --yes --auto-remove curl gnupg && rm -rf /var/lib/apt/lists/* /tmp/rocm.gpg.key
RUN getent group video >/dev/null || groupadd --system video
RUN getent group render >/dev/null || groupadd --system render
RUN groupadd --gid 10001 tensors && useradd --no-create-home --uid 10001 --gid tensors tensors && install -d -o tensors -g tensors /config /models /data /data/vllm
ENV PATH="/opt/rocm/bin:${PATH}"
ENV LD_LIBRARY_PATH="/opt/rocm/lib:/opt/rocm/lib64"
WORKDIR /data
STOPSIGNAL SIGTERM

FROM runtime-cpu AS node
COPY --from=go-builder-musl /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-musl /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime-cpu AS webui
COPY --from=go-builder-musl /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-musl /output/tensor-router-webui /usr/local/bin/tensor-router-webui
COPY --from=go-builder-musl /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]

FROM runtime-vllm AS vllm-node
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
COPY --from=go-builder-glibc /output/tensor-router-vllm /usr/local/bin/tensor-router-vllm
VOLUME ["/data/vllm", "/models"]
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime-vllm AS vllm-webui
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-webui /usr/local/bin/tensor-router-webui
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
COPY --from=go-builder-glibc /output/tensor-router-vllm /usr/local/bin/tensor-router-vllm
VOLUME ["/data/vllm", "/models"]
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]

FROM runtime-cuda AS node-cuda
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime-cuda AS webui-cuda
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-webui /usr/local/bin/tensor-router-webui
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]

FROM runtime-cuda AS vllm-node-cuda
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
COPY --from=go-builder-glibc /output/tensor-router-vllm /usr/local/bin/tensor-router-vllm
VOLUME ["/data/vllm", "/models"]
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime-cuda AS vllm-webui-cuda
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-webui /usr/local/bin/tensor-router-webui
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
COPY --from=go-builder-glibc /output/tensor-router-vllm /usr/local/bin/tensor-router-vllm
VOLUME ["/data/vllm", "/models"]
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]

FROM runtime-rocm AS node-rocm
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime-rocm AS webui-rocm
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-webui /usr/local/bin/tensor-router-webui
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]

FROM runtime-rocm AS vllm-node-rocm
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
COPY --from=go-builder-glibc /output/tensor-router-vllm /usr/local/bin/tensor-router-vllm
VOLUME ["/data/vllm", "/models"]
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime-rocm AS vllm-webui-rocm
COPY --from=go-builder-glibc /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder-glibc /output/tensor-router-webui /usr/local/bin/tensor-router-webui
COPY --from=go-builder-glibc /output/tensor-router-downloader /usr/local/bin/tensor-router-downloader
COPY --from=go-builder-glibc /output/tensor-router-vllm /usr/local/bin/tensor-router-vllm
VOLUME ["/data/vllm", "/models"]
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]
