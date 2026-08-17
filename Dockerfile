FROM cgr.dev/chainguard/go:latest AS builder
ENV CGO_ENABLED=0
WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN go build .

FROM ghcr.io/anchore/syft:latest AS sbomgen
COPY --from=builder /workspace/orbiter /usr/bin/orbiter
RUN ["/syft", "--output", "spdx-json=/tmp/orbiter.spdx.json", "/usr/bin/orbiter"]

FROM cgr.dev/chainguard/static:latest
WORKDIR /tmp
COPY --from=builder /workspace/orbiter /usr/bin/
COPY --from=sbomgen /tmp/orbiter.spdx.json /var/lib/db/sbom/orbiter.spdx.json
ENTRYPOINT ["/usr/bin/orbiter"]
LABEL org.opencontainers.image.title="orbiter"
LABEL org.opencontainers.image.description="Terminal-first Core-Satellite portfolio manager & TAA signal tracker 🛰️"
LABEL org.opencontainers.image.url="https://github.com/tgragnato/orbiter/"
LABEL org.opencontainers.image.source="https://github.com/tgragnato/orbiter/"
LABEL org.opencontainers.image.licenses="AGPL-3.0"
LABEL io.containers.autoupdate=registry
