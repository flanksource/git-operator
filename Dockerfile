# Build the manager binary
FROM golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

# Build libgit2 binary
RUN apt-get update && apt-get -q -y install \
	git openssl apt-transport-https ca-certificates curl g++ gcc libc6-dev make pkg-config \
	libssl-dev cmake && \
    rm -rf /var/lib/apt/lists/*
RUN cd / && \
    git clone -b 'v31.4.14' --single-branch --depth 1 https://github.com/libgit2/git2go.git && \
    cd /git2go && \
    git submodule update --init && \
    make install-static

# Replace remote libgit2 with the local build of libgit2
RUN go mod edit -replace github.com/libgit2/git2go/v31=/git2go
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY connectors/ connectors/
COPY controllers/ controllers/

# Build
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -tags static -a -o manager main.go

FROM golang:1.16
WORKDIR /
COPY --from=builder /workspace/manager ./git-operator

ENTRYPOINT ["/git-operator"]
