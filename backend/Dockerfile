FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/exchangely ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/exchangely /app/exchangely
COPY --from=builder /app/migrations /app/migrations
COPY --from=builder /app/configs /app/configs

EXPOSE 8080

ENTRYPOINT ["/app/exchangely"]
