FROM golang:1.24-alpine AS builder

WORKDIR /build
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /quotaforge ./cmd/api

# ---- Runtime ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /quotaforge .
COPY migrations/ migrations/

EXPOSE 8080

ENV GIN_MODE=release
ENTRYPOINT ["/app/quotaforge"]
