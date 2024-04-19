# syntax = docker/dockerfile-upstream:1.7.0-labs

# THIS FILE WAS AUTOMATICALLY GENERATED, PLEASE DO NOT EDIT.
#
# Generated on 2024-05-06T07:52:18Z by kres d15226e-dirty.

ARG TOOLCHAIN

# cleaned up specs and compiled versions
FROM scratch AS generate

# base toolchain image
FROM ${TOOLCHAIN} AS toolchain
RUN apk --update --no-cache add bash curl build-base protoc protobuf-dev cdrkit cryptsetup dosfstools e2fsprogs parted util-linux xfsprogs lvm2

# build tools
FROM --platform=${BUILDPLATFORM} toolchain AS tools
ENV GO111MODULE on
ARG CGO_ENABLED
ENV CGO_ENABLED ${CGO_ENABLED}
ARG GOTOOLCHAIN
ENV GOTOOLCHAIN ${GOTOOLCHAIN}
ARG GOEXPERIMENT
ENV GOEXPERIMENT ${GOEXPERIMENT}
ENV GOPATH /go
ARG DEEPCOPY_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install github.com/siderolabs/deep-copy@${DEEPCOPY_VERSION} \
	&& mv /go/bin/deep-copy /bin/deep-copy
ARG GOLANGCILINT_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCILINT_VERSION} \
	&& mv /go/bin/golangci-lint /bin/golangci-lint
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install golang.org/x/vuln/cmd/govulncheck@latest \
	&& mv /go/bin/govulncheck /bin/govulncheck
ARG GOIMPORTS_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION} \
	&& mv /go/bin/goimports /bin/goimports
ARG GOFUMPT_VERSION
RUN go install mvdan.cc/gofumpt@${GOFUMPT_VERSION} \
	&& mv /go/bin/gofumpt /bin/gofumpt

# Creates the ZFS image
FROM fedora:39 AS zfs-img-gen
RUN dnf install -y zfs-fuse && rm -rf /var/cache/dnf
RUN --security=insecure zfs-fuse & \
dd if=/dev/zero of=/tmp/zfs.img bs=16M count=4 iflag=fullblock && \
sleep 1 && \
zpool create -f -R /tmp/zfs zroot1 /tmp/zfs.img


# tools and sources
FROM tools AS base
WORKDIR /src
COPY go.mod go.mod
COPY go.sum go.sum
RUN cd .
RUN --mount=type=cache,target=/go/pkg go mod download
RUN --mount=type=cache,target=/go/pkg go mod verify
COPY ./blkid ./blkid
COPY ./block ./block
RUN --mount=type=cache,target=/go/pkg go list -mod=readonly all >/dev/null

# copies out the ZFS image
FROM scratch AS zfs-img
COPY --from=zfs-img-gen /tmp/zfs.img /

# runs gofumpt
FROM base AS lint-gofumpt
RUN FILES="$(gofumpt -l .)" && test -z "${FILES}" || (echo -e "Source code is not formatted with 'gofumpt -w .':\n${FILES}"; exit 1)

# runs goimports
FROM base AS lint-goimports
RUN FILES="$(goimports -l -local github.com/siderolabs/go-blockdevice/v2/ .)" && test -z "${FILES}" || (echo -e "Source code is not formatted with 'goimports -w -local github.com/siderolabs/go-blockdevice/v2/ .':\n${FILES}"; exit 1)

# runs golangci-lint
FROM base AS lint-golangci-lint
WORKDIR /src
COPY .golangci.yml .
ENV GOGC 50
RUN golangci-lint config verify --config .golangci.yml
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/root/.cache/golangci-lint --mount=type=cache,target=/go/pkg golangci-lint run --config .golangci.yml

# runs govulncheck
FROM base AS lint-govulncheck
WORKDIR /src
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg govulncheck ./...

# runs unit-tests with race detector
FROM base AS unit-tests-race
COPY --from=zfs-img / /usr/share/
WORKDIR /src
ARG TESTPKGS
RUN --security=insecure --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg --mount=type=cache,target=/tmp CGO_ENABLED=1 go test -v -race -count 1 ${TESTPKGS}

# runs unit-tests
FROM base AS unit-tests-run
COPY --from=zfs-img / /usr/share/
WORKDIR /src
ARG TESTPKGS
RUN --security=insecure --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg --mount=type=cache,target=/tmp go test -v -covermode=atomic -coverprofile=coverage.txt -coverpkg=${TESTPKGS} -count 1 ${TESTPKGS}

FROM scratch AS unit-tests
COPY --from=unit-tests-run /src/coverage.txt /coverage-unit-tests.txt

