FROM golang:1.26.2-bookworm

RUN apt-get update && \
    apt-get install -y --no-install-recommends bubblewrap python3 ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

ENV ZGI_SANDBOX_TEST_SECURE_ROOTFS=/
ENV ZGI_SANDBOX_TEST_BWRAP_BINARY=bwrap

CMD ["bash"]
