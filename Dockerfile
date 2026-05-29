FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache gcc musl-dev

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o poleia ./cmd/server

FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/poleia ./poleia
COPY --from=builder /build/db ./db
COPY web/ ./web/

ENV STATIC_DIR=/app/web/static
ENV TEMPLATE_DIR=/app/web/templates

EXPOSE 8080
ENTRYPOINT ["./poleia"]
