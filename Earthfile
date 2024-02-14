VERSION 0.7
FROM golang:1.21-bookworm
WORKDIR /workspace

docker-all:
  BUILD --platform=linux/amd64 --platform=linux/arm64 +docker

docker:
  FROM quay.io/ceph/ceph:v18.2
  ARG TARGETARCH
  RUN yum install -y qemu-img
  COPY (+build/picoceph --GOARCH=${TARGETARCH}) /usr/bin/picoceph
  EXPOSE 7480/tcp
  ENTRYPOINT ["picoceph"]
  ARG VERSION=latest-dev
  SAVE IMAGE --push ghcr.io/bucket-sailor/picoceph:${VERSION}
  SAVE IMAGE --push ghcr.io/bucket-sailor/picoceph:latest

build:
  ARG GOOS=linux
  ARG GOARCH=amd64
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN CGO_ENABLED=0 go build --ldflags '-s' -o picoceph cmd/main.go
  SAVE ARTIFACT ./picoceph AS LOCAL dist/picoceph-${GOOS}-${GOARCH}

tidy:
  LOCALLY
  RUN go mod tidy
  RUN go fmt ./...

lint:
  FROM golangci/golangci-lint:v1.55.2
  WORKDIR /workspace
  COPY . ./
  RUN golangci-lint run --timeout 5m ./...

test:
  COPY . ./
  RUN go test -coverprofile=coverage.out -v ./...
  SAVE ARTIFACT ./coverage.out AS LOCAL coverage.out