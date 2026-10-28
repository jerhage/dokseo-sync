FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X github.com/jerhage/dokseo-sync/internal/api.Version=${VERSION}" -o /out/dokseo-sync ./cmd/dokseo-sync
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/jerhage/dokseo-sync"
LABEL org.opencontainers.image.description="Sync server for Dokseo"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /out/dokseo-sync /usr/local/bin/dokseo-sync
COPY --from=build --chown=65532:65532 /out/data /data
ENV DOKSEO_SYNC_LISTEN=0.0.0.0:8080
ENV DOKSEO_SYNC_DATA=/data
VOLUME /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/dokseo-sync"]
