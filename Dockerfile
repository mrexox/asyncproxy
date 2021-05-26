FROM golang:1.16-alpine as builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags '-w -s' -o /app/asyncproxy .

FROM scratch

COPY --from=builder /app/asyncproxy /asyncproxy
COPY --from=builder /app/config.yaml /config.yaml

ENTRYPOINT ["/asyncproxy"]
