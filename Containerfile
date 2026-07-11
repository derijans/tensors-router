ARG GO_VERSION=1.26.5
ARG NODE_VERSION=24-alpine

FROM node:${NODE_VERSION} AS web-assets
WORKDIR /source
COPY webui/package.json webui/package-lock.json ./webui/
RUN npm --prefix webui ci
COPY webui ./webui
COPY internal/webui/assets ./internal/webui/assets
RUN npm --prefix webui run build

FROM golang:${GO_VERSION}-alpine AS go-builder
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

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 tensors && adduser -D -H -u 10001 -G tensors tensors && mkdir -p /config /models /data && chown -R tensors:tensors /data
WORKDIR /data
STOPSIGNAL SIGTERM

FROM runtime AS node
COPY --from=go-builder /output/tensors-router /usr/local/bin/tensors-router
USER tensors
EXPOSE 8080
ENTRYPOINT ["tensors-router", "serve", "--config", "/config/config.yaml"]

FROM runtime AS webui
COPY --from=go-builder /output/tensors-router /usr/local/bin/tensors-router
COPY --from=go-builder /output/tensor-router-webui /usr/local/bin/tensor-router-webui
USER tensors
EXPOSE 8080 8443 8444
ENTRYPOINT ["tensor-router-webui", "--config", "/config/webui.yaml"]
