FROM golang:1.16-alpine as builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags '-w -s' -o /app/proxy .

FROM scratch

COPY --from=builder /app/proxy /proxy
COPY --from=builder /app/config.yaml /config.yaml

ENTRYPOINT ["/proxy"]
