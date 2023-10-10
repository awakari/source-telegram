FROM alpine:3.18.3

ENV LANG en_US.UTF-8
ENV TZ UTC

ARG TD_COMMIT

RUN apk update && \
    apk upgrade && \
    apk add --update \
        build-base \
        ca-certificates \
        ccache \
        cmake \
        git \
        gperf \
        linux-headers \
        openssl-dev \
        php \
        php-ctype \
        readline-dev \
        zlib-dev && \
    git clone "https://github.com/tdlib/td.git" /src && \
    cd /src && \
    git checkout ${TD_COMMIT} && \
    mkdir ./build && \
    cd ./build && \
    cmake \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX:PATH=/usr/local \
        .. && \
    cmake --build . --target prepare_cross_compiling && \
    cd .. && \
    php SplitSource.php && \
    cd build && \
    cmake --build . --target install && \
    ls -lah /usr/local
