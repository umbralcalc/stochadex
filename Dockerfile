# syntax=docker/dockerfile:1

# The stochadex CLI as an OCI image — the unit cloud-native pipelines actually
# compose (Kubernetes Jobs, Argo steps, Cloud Run Jobs), and the natural way to
# run the websocket service mode.
#
# This builds the FULLY ACCELERATED CLI: cmd/stochadex-full with
# -tags "cblas duckdb_arrow", so the image carries Arrow, Postgres, S3, DuckDB and
# an optimised system BLAS. There is no portable/accelerated split here on purpose.
# That split exists because a *binary* has to survive whatever host it lands on —
# cgo cannot cross-compile, and neither OpenBLAS nor DuckDB can be assumed present.
# An image has no such problem: it carries its own userland, so the reason for the
# lesser tier evaporates and shipping it would only mean a container advertised for
# pipeline chaining that lacks the egress pipelines chain through.
#
# The image carries the config-as-data path ONLY. It runs any fully-data config and
# any macros: config — the whole {type: ...} registry, the expressions DSL, run
# modes and the analysis tier — resolved and stepped in-process. It deliberately
# ships no Go toolchain: configs naming Go expressions are executed by generating a
# program and calling `go run` (pkg/api/run.go), and shipping a compiler in a
# runtime image would mean a far larger image and arbitrary compilation at run time,
# to serve the surface the engine is deliberately moving away from. That path stays
# a local development affordance, where a toolchain already exists.

# cmd/stochadex-full declares `go 1.25.0` — it will not build on an older toolchain,
# and it is a SEPARATE module whose replace directives point at ../../ and
# ../../pkg/*, so the whole repo has to be in the build context.
ARG GO_VERSION=1.25
ARG DEBIAN_RELEASE=bookworm

# ---- build -------------------------------------------------------------------
FROM golang:${GO_VERSION}-${DEBIAN_RELEASE} AS build

# libopenblas-dev provides the BLAS the `cblas` tag links gonum against.
RUN apt-get update \
 && apt-get install -y --no-install-recommends libopenblas-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
# The local replace directives mean module resolution needs the sibling modules
# present, so the whole context is copied before download rather than the usual
# go.mod-only cache layer.
COPY . .

ARG VERSION=dev
RUN cd cmd/stochadex-full \
 && CGO_ENABLED=1 CGO_LDFLAGS="-lopenblas" go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -tags "cblas duckdb_arrow" \
      -o /out/stochadex .

# Fail the build here rather than ship an image that silently lost an integration:
# a dropped build tag or a missing library would otherwise only show up as a
# config that mysteriously cannot write its output.
RUN /out/stochadex --version \
 && for feature in arrow postgres s3 cblas duckdb; do \
      /out/stochadex --version | grep -q "$feature" \
        || { echo "BUILD LOST FEATURE: $feature"; /out/stochadex --version; exit 1; }; \
    done

# ---- runtime -----------------------------------------------------------------
# Debian slim rather than distroless: cgo needs a libc, and OpenBLAS is linked
# dynamically. Matching the builder's Debian release keeps the glibc and OpenBLAS
# ABI identical to what the binary was linked against.
FROM debian:${DEBIAN_RELEASE}-slim

# Exactly what `ldd` reports the accelerated binary linking, named explicitly:
#   libopenblas0  the BLAS the cblas tag links against
#   libgfortran5  OpenBLAS's Fortran runtime — it arrives transitively today, but
#                 the binary links it directly, so relying on that is fragile
#   libstdc++6    DuckDB's C++ runtime
# ca-certificates is not a link-time dependency and so does not appear in ldd, but
# it is not optional either: S3 egress and any HTTPS data source fail without it,
# which is the kind of omission that only surfaces in production.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      libopenblas0 libgfortran5 libstdc++6 ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/stochadex /usr/local/bin/stochadex

# The websocket port used by cfg/socket.yaml when --socket is passed.
EXPOSE 2112

# Configs and any CSV/JSON egress are expected under /work, so a caller only has
# to mount one directory.
WORKDIR /work

# Non-root by default; a simulation never needs privilege.
RUN useradd --uid 65532 --user-group --home-dir /work --no-create-home stochadex \
 && chown stochadex:stochadex /work
USER stochadex

ENTRYPOINT ["stochadex"]
