# syntax=docker/dockerfile:1

# The stochadex CLI as an OCI image — the unit cloud-native pipelines actually
# compose (Kubernetes Jobs, Argo steps, Cloud Run Jobs), and the natural way to
# run the websocket service mode.
#
# The image carries the config-as-data path ONLY. It runs any fully-data config
# and any macros: config — the whole {type: ...} registry, the expressions DSL,
# run modes and the analysis tier — resolved and stepped in-process.
#
# It deliberately does NOT ship a Go toolchain. Configs that name Go expressions
# are executed by generating a program and running `go run` on it
# (pkg/api/run.go), and shipping a compiler in a runtime image to serve that path
# would mean a ~900MB image, a compiler in the production attack surface, and
# arbitrary code compilation at run time — to support the surface the engine is
# deliberately moving away from. A container is for the declarative path; the Go
# path stays a local development affordance, where a toolchain already exists.
#
# Also deliberately pure Go (CGO_ENABLED=0) rather than the accelerated
# cmd/stochadex-full tier: the accelerated build needs BLAS and DuckDB linked
# per-platform, and the egress that pipeline chaining actually leans on
# (Postgres, Arrow, S3) is all in the portable build. An accelerated variant is
# worth adding once this pipeline has proven itself, not before.

ARG GO_VERSION=1.24.4

# ---- build -------------------------------------------------------------------
FROM golang:${GO_VERSION} AS build
WORKDIR /src

# Module download is its own layer so it is not invalidated by source edits.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/stochadex ./cmd/stochadex

# ---- runtime -----------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/stochadex /usr/local/bin/stochadex

# The websocket port used by cfg/socket.yaml when --socket is passed.
EXPOSE 2112

# Configs and any CSV/JSON egress are expected under /work, so a caller only has
# to mount one directory.
WORKDIR /work
USER nonroot:nonroot
ENTRYPOINT ["stochadex"]
