FROM golang:1.17-alpine as builder
ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct
ARG GOOS=linux
ARG GOARCH=amd64
WORKDIR /src
COPY . .
RUN go mod download
RUN mkdir -p /src/.build && \
    cd /src/cmd && \
    go build -o ../.build/discovery-syncer-${GOOS}-${GOARCH}


FROM alpine:3.14
LABEL maintainer="AnJia <anjia0532@gmail.com>"

ARG GOARCH="amd64"
ARG GOOS="linux"
COPY --from=builder /src/.build/discovery-syncer-${GOOS}-${GOARCH}       /bin/discovery-syncer
COPY config-example.yaml                                                 /etc/discovery-syncer/config.yml

RUN mkdir -p /discovery-syncer && \
    chown -R nobody:nobody /etc/discovery-syncer /discovery-syncer

USER       nobody
EXPOSE     8080
VOLUME     [ "/discovery-syncer" ]
WORKDIR    /discovery-syncer
ENTRYPOINT [ "/bin/discovery-syncer" ]
CMD        [ "--config.file=/etc/discovery-syncer/config.yml" ]
