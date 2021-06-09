FROM golang:1.16-alpine as builder

WORKDIR /app

COPY . .

RUN apk add --no-cache sqlite-libs sqlite-dev gcc libc-dev
RUN CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags '-w -s' -o /app/asyncproxy .

FROM alpine:3.4

RUN apk add --no-cache sqlite-libs

COPY --from=builder /app/asyncproxy /asyncproxy
COPY --from=builder /app/config.yaml /config.yaml

RUN mkdir /request_db

CMD ["/asyncproxy"]
