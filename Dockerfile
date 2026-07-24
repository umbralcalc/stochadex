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
# The whole config surface is data: any config — the {type: ...} registry, the
# expressions DSL, run modes and the analysis tier — resolves and steps in-process,
# so the image ships no Go toolchain and needs none. Genuinely bespoke Go iterations
# live in a downstream repo that embeds the engine as a library, not in a config.

# cmd/stochadex-full declares `go 1.25.0` — it will not build on an older toolchain,
# and it is a SEPARATE module whose replace directives point at ../../ and
# ../../pkg/*, so the whole repo has to be in the build context.
ARG GO_VERSION=1.25
ARG DEBIAN_RELEASE=bookworm

# VERSION is the human tag (e.g. 0.7.0); REVISION is the git commit it was built from.
# Both are passed by the release workflow and become OCI labels + a compiled-in
# version stamp, so a pulled image can be traced back to an exact source commit.
ARG VERSION=dev
ARG REVISION=unknown

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

# .git is excluded from the build context (see .dockerignore), so the toolchain
# cannot embed the revision itself — it is stamped in explicitly from the REVISION
# build-arg instead, and the run provenance line reports it (api.BuildRevision).
ARG VERSION=dev
ARG REVISION=unknown
RUN cd cmd/stochadex-full \
 && CGO_ENABLED=1 CGO_LDFLAGS="-lopenblas" go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.revision=${REVISION}" \
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

# OCI provenance labels, fed from the build-args. org.opencontainers.image.source is
# the load-bearing one: it is what makes GHCR link this package back to the repository
# and what a registry UI reads to show where an image came from. revision + version
# pin the exact source a pulled image was built from — the provenance the release
# workflow's SBOM/attestation complement rather than replace.
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="stochadex" \
      org.opencontainers.image.description="Accelerated stochadex CLI — data-path simulation engine for cloud-native pipelines" \
      org.opencontainers.image.source="https://github.com/umbralcalc/stochadex" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

# The same version/revision as env vars, so a running container can report its own
# provenance without shelling out to inspect labels. STOCHADEX_IMAGE_DIGEST is left
# UNSET on purpose: the digest is a hash of this image and so cannot be known while
# building it. A deployer that resolved an image@sha256:… reference passes that digest
# in via this variable, and the run provenance line (api.LogRunProvenance) echoes it.
ENV STOCHADEX_VERSION="${VERSION}" \
    STOCHADEX_REVISION="${REVISION}"

# The websocket port used by cfg/socket.yaml when --socket is passed.
EXPOSE 2112

# Configs and any CSV/JSON egress are expected under /work, so a caller only has
# to mount one directory.
WORKDIR /work

# Non-root by default; a simulation never needs privilege. UID 65532 is the
# conventional "nonroot" id (matches distroless), so it is predictable for wrappers
# that pre-create or chown a mount.
#
# BIND-MOUNT CONTRACT: because the process runs as 65532, a host directory mounted at
# /work for output must be writable by that uid, or CSV/JSON egress fails with
# "permission denied". On Docker Desktop (macOS/Windows) the file-sharing layer maps
# ownership and this is invisible; on native Linux it is not. Two escape hatches:
#   - run as the host user:   docker run --user "$(id -u):$(id -g)" -v "$PWD:/work" …
#   - or pre-chown the mount:  chown 65532:65532 <output-dir>
# Read-only inputs never hit this — only directories the run writes into.
RUN useradd --uid 65532 --user-group --home-dir /work --no-create-home stochadex \
 && chown stochadex:stochadex /work
USER stochadex

ENTRYPOINT ["stochadex"]
