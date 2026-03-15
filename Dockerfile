FROM golang:1.26-alpine AS builder

ARG VERSION=dev

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w -X github.com/huseyinbabal/updock/internal/config.Version=${VERSION}" -o /updock ./cmd/updock

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /updock /usr/local/bin/updock

EXPOSE 8080

ENTRYPOINT ["updock"]
