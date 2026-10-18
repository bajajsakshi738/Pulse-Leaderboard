# --- Build stage ---
FROM golang:1.22-bookworm AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum* ./
RUN go mod download

COPY . .

# Build both binaries. CGO disabled for a fully static binary that
# runs in a minimal distroless/alpine runtime image.
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/generator ./cmd/generator

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bin/server ./server
COPY --from=builder /app/bin/generator ./generator
COPY migrations ./migrations

EXPOSE 8080

# Default command runs the API server. Render's generator background
# worker service overrides this with `./generator` via its own start
# command (see render.yaml).
CMD ["./server"]
