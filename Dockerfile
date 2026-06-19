FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server
# photos: one-off backfill tool that fills points.photos from image search.
# Shipped in the image so it can be run via `docker compose run --rm app ./photos`.
RUN CGO_ENABLED=0 GOOS=linux go build -o photos ./cmd/photos

# ---

FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/photos .

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./server"]
