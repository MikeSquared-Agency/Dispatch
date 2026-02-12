FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /dispatch ./cmd/dispatch

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /dispatch /usr/local/bin/dispatch
EXPOSE 8600 8601
ENTRYPOINT ["dispatch"]
