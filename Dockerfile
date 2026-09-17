# syntax=docker/dockerfile:1

# ---- build stage -----------------------------------------------------------
FROM golang:1.27-alpine AS build

WORKDIR /src

# Cache module downloads separately from the sources.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/homey \
    ./cmd/server

# ---- runtime stage ---------------------------------------------------------
FROM alpine:3.22

# Run as a non-root user; /data holds the SQLite database.
RUN adduser -D -u 1000 homey \
    && mkdir -p /data \
    && chown homey:homey /data

COPY --from=build /out/homey /usr/local/bin/homey

ENV HOMEY_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080

USER homey
ENTRYPOINT ["homey"]
CMD ["serve"]
