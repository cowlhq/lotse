# syntax=docker/dockerfile:1
# One image for every mode. It runs the starter (decision 0009), which runs
# the newest version in /var/lib/lotse/bin, or the binary in this image.
#
#   docker run ghcr.io/cowlhq/lotse                  # the hub
#   docker run ghcr.io/cowlhq/lotse start agent …    # an agent

FROM --platform=$BUILDPLATFORM node:24-slim AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY web ./
RUN mkdir -p ../internal/ui/dist && pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd cmd
COPY internal internal
COPY api api
COPY client client
COPY --from=web /src/internal/ui/dist internal/ui/dist
ARG TARGETOS TARGETARCH VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X lotse.sh/internal/version.Version=$VERSION" \
      -o /out/lotse ./cmd/lotse \
 && mkdir -p /out/data/bin

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/lotse /usr/local/bin/lotse
# A named volume mounted here starts out owned by the user lotse runs as.
COPY --from=build --chown=nonroot:nonroot /out/data /var/lib/lotse
ENV LOTSE_DATA_DIR=/var/lib/lotse
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/lotse"]
CMD ["start", "hub"]
