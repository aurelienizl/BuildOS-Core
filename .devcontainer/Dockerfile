# Match the Go version and Debian base used by the test builder.
FROM docker.io/library/golang:1.22-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    make \
    && rm -rf /var/lib/apt/lists/*
