# renovate: datasource=github-releases depName=amadvance/snapraid
ARG SNAPRAID_VERSION=14.9

FROM golang:1.26-alpine AS exporter
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /snapraid-exporter .

FROM alpine:3 AS snapraid
ARG SNAPRAID_VERSION
RUN apk add --no-cache build-base curl
RUN curl -fsSL "https://github.com/amadvance/snapraid/releases/download/v${SNAPRAID_VERSION}/snapraid-${SNAPRAID_VERSION}.tar.gz" | tar xz -C /tmp \
 && cd /tmp/snapraid-${SNAPRAID_VERSION} \
 && ./configure --prefix=/usr && make -j"$(nproc)" && make DESTDIR=/out install

FROM alpine:3
# smartmontools: snapraid smart shells out to smartctl.
RUN apk add --no-cache smartmontools
COPY --from=snapraid /out/usr/bin/snapraid /usr/bin/snapraid
COPY --from=exporter /snapraid-exporter /usr/local/bin/snapraid-exporter
EXPOSE 9634
ENTRYPOINT ["/usr/local/bin/snapraid-exporter"]
CMD ["--web.listen-address=:9634"]
