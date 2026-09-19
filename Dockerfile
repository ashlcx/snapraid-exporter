FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /snapraid-exporter .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /snapraid-exporter /snapraid-exporter
EXPOSE 9634
ENTRYPOINT ["/snapraid-exporter"]
