FROM golang:1.26.1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -mod=mod -o /out/exchangely ./cmd/server

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /out/exchangely /app/exchangely
COPY --from=builder /app/migrations /app/migrations
COPY --from=builder /app/configs /app/configs

EXPOSE 8080

ENTRYPOINT ["/app/exchangely"]
