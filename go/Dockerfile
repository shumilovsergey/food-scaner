# Build context: repo root (../)
# docker-compose sets context: ..

FROM golang:1.22-bookworm AS builder
WORKDIR /build

COPY go/go.mod ./go.mod
ENV GONOSUMDB=* GOFLAGS=-mod=mod

COPY go/*.go ./
COPY go/static/ ./static/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o foodscaner .

FROM scratch AS binary
COPY --from=builder /build/foodscaner .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/foodscaner .
EXPOSE 8080
ENTRYPOINT ["./foodscaner"]
