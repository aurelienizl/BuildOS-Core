CONTAINER_ENGINE ?= docker
FORMAT_USER = $(if $(filter podman,$(notdir $(CONTAINER_ENGINE))),--userns=keep-id,--user "$$(id -u):$$(id -g)")
export CONTAINER_ENGINE

.PHONY: all build fmt test test-containers coverage
all: build

build:
	$(CONTAINER_ENGINE) build --target build -f tests/Dockerfile -t buildos-build .
	@trap 'rm -f buildos.tmp' EXIT; $(CONTAINER_ENGINE) run --rm --entrypoint cat buildos-build /out/buildos > buildos.tmp && chmod 755 buildos.tmp && mv buildos.tmp buildos

fmt:
	$(CONTAINER_ENGINE) run --rm $(FORMAT_USER) -v "$(CURDIR):/work" -w /work docker.io/library/golang:1.22-bookworm gofmt -w src tests

test:
	bash tests/containers.sh $(TARGET)

test-containers: test

coverage:
	COVERAGE=1 bash tests/containers.sh $(TARGET)
