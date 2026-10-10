FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache dependencies before copying source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /api-service ./cmd/api

# ── Final image ────────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /api-service /usr/local/bin/api-service

EXPOSE 8080
USER nobody

CMD ["api-service"]
