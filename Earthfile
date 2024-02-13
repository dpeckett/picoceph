VERSION 0.7
FROM debian:bookworm
WORKDIR /workspace

docker-all:
  BUILD --platform=linux/amd64 --platform=linux/arm64 +docker

docker:
  ARG TARGETARCH
  RUN apt update
  RUN apt install -y ceph ceph-common radosgw qemu-utils kmod udev
  COPY ceph.conf /etc/ceph/ceph.conf
  RUN ceph-authtool --create-keyring /tmp/ceph.mon.keyring --gen-key -n mon. --cap mon 'allow *' \
    && ceph-authtool --create-keyring /etc/ceph/ceph.client.admin.keyring --gen-key -n client.admin --cap mon 'allow *' --cap osd 'allow *' --cap mds 'allow *' --cap mgr 'allow *' \
    && ceph-authtool --create-keyring /var/lib/ceph/bootstrap-osd/ceph.keyring --gen-key -n client.bootstrap-osd --cap mon 'profile bootstrap-osd' --cap mgr 'allow r' \
    && ceph-authtool /tmp/ceph.mon.keyring --import-keyring /etc/ceph/ceph.client.admin.keyring \
    && ceph-authtool /tmp/ceph.mon.keyring --import-keyring /var/lib/ceph/bootstrap-osd/ceph.keyring \
    && chown ceph:ceph /tmp/ceph.mon.keyring \
    && monmaptool --create --addv a '[v2:127.0.0.1:3300,v1:127.0.0.1:6789]' --fsid 2b82f0c5-4ab2-4e20-b641-5700c2247feb /tmp/monmap \
    && ceph-mon --mkfs -i a --monmap /tmp/monmap --keyring /tmp/ceph.mon.keyring \
    && chown -R ceph:ceph /var/lib/ceph /etc/ceph
  # Keep ceph-volume happy.
  RUN ln -s /bin/true /usr/bin/systemctl
  COPY (+build/picoceph --GOARCH=${TARGETARCH}) /usr/bin/picoceph
  EXPOSE 7480/tcp
  ENTRYPOINT ["picoceph"]
  ARG VERSION=latest-dev
  SAVE IMAGE --push ghcr.io/bucket-sailor/picoceph:${VERSION}
  SAVE IMAGE --push ghcr.io/bucket-sailor/picoceph:latest

build:
  FROM golang:1.21-bookworm
  WORKDIR /workspace
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
  FROM golang:1.21-bookworm
  WORKDIR /workspace
  COPY . ./
  RUN go test -coverprofile=coverage.out -v ./...
  SAVE ARTIFACT ./coverage.out AS LOCAL coverage.out