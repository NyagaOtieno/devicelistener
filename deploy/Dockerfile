FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
ARG SERVICE=gt06-listener
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/service ./cmd/${SERVICE}

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/service /app/service
CMD ["/app/service"]
