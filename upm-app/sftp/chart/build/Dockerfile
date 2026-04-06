FROM atmoz/sftp:latest

USER root

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        netcat-openbsd \
        nfs-common \
        rpcbind \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*
