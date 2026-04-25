# syntax = docker/dockerfile-upstream:1.23.0-labs

# THIS FILE WAS AUTOMATICALLY GENERATED, PLEASE DO NOT EDIT.
#
# Generated on 2026-05-07T15:09:05Z by kres 1762ab2.

ARG TOOLCHAIN=scratch

# runs markdownlint
FROM docker.io/oven/bun:1.3.13-alpine AS lint-markdown
WORKDIR /src
RUN bun i markdownlint-cli@0.48.0 sentences-per-line@0.5.2
COPY .markdownlint.json .
COPY ./CHANGELOG.md ./CHANGELOG.md
RUN bunx markdownlint --ignore "CHANGELOG.md" --ignore "**/node_modules/**" --ignore '**/hack/chglog/**' --rules markdownlint-sentences-per-line .

# base toolchain image
FROM --platform=${BUILDPLATFORM} ${TOOLCHAIN} AS toolchain
RUN apk --update --no-cache add bash build-base curl jq protoc protobuf-dev btrfs-progs cdrkit cryptsetup dosfstools e2fsprogs gptfdisk lvm2 parted util-linux squashfs-tools xfsprogs mtools

# Creates the ZFS image
FROM fedora:39 AS zfs-img-gen
RUN dnf install -y zfs-fuse zstd && rm -rf /var/cache/dnf
RUN --security=insecure zfs-fuse & \
dd if=/dev/zero of=/tmp/zfs.img bs=16M count=4 iflag=fullblock && \
sleep 1 && \
zpool create -f -R /tmp/zfs zroot1 /tmp/zfs.img && \
zstd -19 /tmp/zfs.img -o /tmp/zfs.img.zst


# build tools
FROM --platform=${BUILDPLATFORM} toolchain AS tools
ENV GO111MODULE=on
ARG CGO_ENABLED
ENV CGO_ENABLED=${CGO_ENABLED}
ARG GOTOOLCHAIN
ENV GOTOOLCHAIN=${GOTOOLCHAIN}
ARG GOEXPERIMENT
ENV GOEXPERIMENT=${GOEXPERIMENT}
ENV GOPATH=/go
ARG GOIMPORTS_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go install golang.org/x/tools/cmd/goimports@v${GOIMPORTS_VERSION}
RUN mv /go/bin/goimports /bin
ARG GOMOCK_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go install go.uber.org/mock/mockgen@v${GOMOCK_VERSION}
RUN mv /go/bin/mockgen /bin
ARG DEEPCOPY_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go install github.com/siderolabs/deep-copy@${DEEPCOPY_VERSION} \
	&& mv /go/bin/deep-copy /bin/deep-copy
ARG GOLANGCILINT_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCILINT_VERSION} \
	&& mv /go/bin/golangci-lint /bin/golangci-lint
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go install golang.org/x/vuln/cmd/govulncheck@latest \
	&& mv /go/bin/govulncheck /bin/govulncheck
ARG DIS_VULNCHECK_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go install github.com/shanduur/dis-vulncheck@${DIS_VULNCHECK_VERSION} \
	&& mv /go/bin/dis-vulncheck /bin/dis-vulncheck
ARG GOFUMPT_VERSION
RUN go install mvdan.cc/gofumpt@${GOFUMPT_VERSION} \
	&& mv /go/bin/gofumpt /bin/gofumpt

# copies out the ZFS image
FROM scratch AS zfs-img
COPY --from=zfs-img-gen /tmp/zfs.img.zst /

# tools and sources
FROM tools AS base
WORKDIR /src
COPY go.mod go.mod
COPY go.sum go.sum
RUN cd .
RUN --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go mod download
RUN --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go mod verify
COPY ./internal ./internal
COPY ./blkid ./blkid
COPY ./block ./block
COPY ./encryption ./encryption
COPY ./partitioning ./partitioning
COPY ./swap ./swap
RUN --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go list -mod=readonly all >/dev/null

# run go generate
FROM base AS go-generate-0
WORKDIR /src
COPY .license-header.go.txt hack/.license-header.go.txt
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg go generate ./...
RUN goimports -w -local github.com/siderolabs/go-blockdevice/v2 .

# runs gofumpt
FROM base AS lint-gofumpt
RUN FILES="$(gofumpt -l .)" && test -z "${FILES}" || (echo -e "Source code is not formatted with 'gofumpt -w .':\n${FILES}"; exit 1)

# runs golangci-lint
FROM base AS lint-golangci-lint
WORKDIR /src
COPY .golangci.yml .
ENV GOGC=50
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/root/.cache/golangci-lint,id=go-blockdevice/root/.cache/golangci-lint,sharing=locked --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg golangci-lint run --config .golangci.yml

# runs golangci-lint fmt
FROM base AS lint-golangci-lint-fmt-run
WORKDIR /src
COPY .golangci.yml .
ENV GOGC=50
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/root/.cache/golangci-lint,id=go-blockdevice/root/.cache/golangci-lint,sharing=locked --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg golangci-lint fmt --config .golangci.yml
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/root/.cache/golangci-lint,id=go-blockdevice/root/.cache/golangci-lint,sharing=locked --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg golangci-lint run --fix --issues-exit-code 0 --config .golangci.yml

# runs govulncheck
FROM base AS lint-govulncheck
WORKDIR /src
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg dis-vulncheck -tool=false ./...

# runs unit-tests with race detector
FROM base AS unit-tests-race
COPY --from=zfs-img / /src/blkid/testdata/
WORKDIR /src
ARG TESTPKGS
RUN --security=insecure --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg --mount=type=cache,target=/tmp,id=go-blockdevice/tmp CGO_ENABLED=1 go test -race ${TESTPKGS}

# runs unit-tests
FROM base AS unit-tests-run
COPY --from=zfs-img / /src/blkid/testdata/
WORKDIR /src
ARG TESTPKGS
RUN --security=insecure --mount=type=cache,target=/root/.cache/go-build,id=go-blockdevice/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=go-blockdevice/go/pkg --mount=type=cache,target=/tmp,id=go-blockdevice/tmp go test -covermode=atomic -coverprofile=coverage.txt -coverpkg=${TESTPKGS} ${TESTPKGS}

# cleaned up specs and compiled versions
FROM scratch AS generate
COPY --from=go-generate-0 /src/blkid blkid
COPY --from=go-generate-0 /src/internal internal

# clean golangci-lint fmt output
FROM scratch AS lint-golangci-lint-fmt
COPY --from=lint-golangci-lint-fmt-run /src .

FROM scratch AS unit-tests
COPY --from=unit-tests-run /src/coverage.txt /coverage-unit-tests.txt

