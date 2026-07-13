FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wa-gateway ./cmd/gateway

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S gateway && adduser -S gateway -G gateway
WORKDIR /app
COPY --from=build /out/wa-gateway /app/wa-gateway
COPY migrations /app/migrations
RUN mkdir -p /app/auth_sessions && chown -R gateway:gateway /app
USER gateway
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:3000/ || exit 1
ENTRYPOINT ["/app/wa-gateway"]
