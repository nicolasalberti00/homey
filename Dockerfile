# syntax=docker/dockerfile:1

# ---- web stage -------------------------------------------------------------
# The SPA is built first: the Go build embeds its output into the binary, so
# the runtime image stays a single binary with no static files.
FROM node:24-alpine AS web

WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ---- build stage -----------------------------------------------------------
FROM golang:1.27-alpine AS build

WORKDIR /src

# Cache module downloads separately from the sources.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The embed tree is filled here; the context never carries a stale local copy.
COPY --from=web /src/web/build/ ./internal/webui/dist/

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
