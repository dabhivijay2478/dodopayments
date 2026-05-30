FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /api /api
EXPOSE 8080
CMD ["/api"]
