FROM golang:1.25-alpine AS builder
# Byggsteget blir OTAGGAT vid varje ombygge och kan därför inte kännas igen på
# namn. Etiketten är det enda som överlever avtaggningen, och den är vad
# tools/acceptance.sh städar på — så riggen kan slänga SINA gamla byggsteg utan
# att röra andra projekts images. Se drop_stale_builder_images där.
LABEL com.megaron.build-stage=builder

WORKDIR /build

RUN apk add --no-cache gcc musl-dev

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o temenos ./cmd/server

FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/temenos ./temenos
COPY --from=builder /build/db ./db
COPY web/ ./web/

ENV STATIC_DIR=/app/web/static
ENV TEMPLATE_DIR=/app/web/templates

EXPOSE 8080
ENTRYPOINT ["./temenos"]
