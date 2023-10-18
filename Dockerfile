FROM golang:1.21.2-alpine3.18 as go-builder

ENV LANG en_US.UTF-8
ENV TZ UTC

RUN set -eux && \
    apk update && \
    apk upgrade && \
    apk add \
        bash \
        build-base \
        ca-certificates \
        curl \
        git \
        linux-headers \
        openssl-dev \
        zlib-dev

WORKDIR /src

COPY --from=ghcr.io/awakari/tdlib:latest /usr/local/include/td /usr/local/include/td/
COPY --from=ghcr.io/awakari/tdlib:latest /usr/local/lib/libtd* /usr/local/lib/
COPY . /src

RUN go build \
    -a \
    -trimpath \
    -ldflags "-s -w" \
    -o app \
    "./main.go" && \
    ls -lah

FROM alpine:3.18.3

ENV LANG en_US.UTF-8
ENV TZ UTC

RUN apk upgrade --no-cache && \
    apk add --no-cache \
            ca-certificates \
            libstdc++

WORKDIR /app

COPY --from=go-builder /src/app .
RUN apk upgrade --no-cache && \
    apk add --no-cache \
        screen

CMD ["sleep", "infinity"]
