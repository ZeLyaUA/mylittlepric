# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api ./cmd/api/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

RUN addgroup -g 1000 appuser && adduser -D -u 1000 -G appuser appuser

WORKDIR /app

COPY --from=builder /app/api .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/internal ./internal

RUN chown -R appuser:appuser /app

USER appuser

CMD ["./api"]
