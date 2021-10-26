FROM alpine:3.14
LABEL maintainer="AnJia <anjia0532@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/discovery-syncer-${OS}-${ARCH}       /bin/discovery-syncer
COPY config-example.yaml                         /etc/discovery-syncer/config.yml

RUN mkdir -p /discovery-syncer && \
    chown -R nobody:nobody /etc/discovery-syncer /discovery-syncer

USER       nobody
EXPOSE     8080
VOLUME     [ "/discovery-syncer" ]
WORKDIR    /discovery-syncer
ENTRYPOINT [ "/bin/discovery-syncer" ]
CMD        [ "--config.file=/etc/discovery-syncer/config.yml" ]
