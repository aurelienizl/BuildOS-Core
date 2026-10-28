ARG TARGET_IMAGE=docker.io/library/debian:12-slim
FROM docker.io/library/golang:1.22-bookworm AS build
WORKDIR /work
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
COPY tests ./tests
ARG COVERAGE=0
RUN test -z "$(gofmt -l src tests)" && go vet ./... && \
    if [ "$COVERAGE" = 1 ]; then CGO_ENABLED=0 go build -cover -coverpkg=buildos/src/... -o /out/buildos ./src/cmd/buildos; \
    else CGO_ENABLED=0 go build -o /out/buildos ./src/cmd/buildos; fi && \
    CGO_ENABLED=0 go test -c -o /out/integration.test ./tests

FROM ${TARGET_IMAGE} AS test
# Prerequisites only: no BuildOS profile is applied to this image.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget sudo systemd && rm -rf /var/lib/apt/lists/*
RUN mkdir /coverage
COPY --from=build /out/ /usr/local/bin/
COPY tests /work/tests
ENV BUILDOS_ROOT=/work BUILDOS_BINARY=/usr/local/bin/buildos BUILDOS_CONTAINER_TESTS=1
ENTRYPOINT ["/usr/local/bin/integration.test"]
CMD ["-test.v"]
