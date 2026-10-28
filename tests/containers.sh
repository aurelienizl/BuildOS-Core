#!/usr/bin/env bash
# Each module profile and regression test gets a fresh disposable container.
set -euo pipefail
cd "$(dirname "$0")/.."
engine=${CONTAINER_ENGINE:-docker}
coverage=${COVERAGE:-0}
targets=("$@")
if ((${#targets[@]} == 0)); then
    targets=(debian:12-slim ubuntu:24.04)
fi
if ! "$engine" info >/dev/null 2>&1; then
    echo "Cannot access $engine. Start an accessible container engine and retry make test." >&2
    exit 1
fi
if [[ -n ${MODULE:-} && ! -f tests/profile/$MODULE.yaml ]]; then
    echo "Unknown module: $MODULE" >&2
    exit 1
fi
container="buildos-test-$$"
trap '"$engine" rm -f "$container" >/dev/null 2>&1 || true' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if [[ $coverage == 1 ]]; then
    mkdir -p coverage
    rm -rf coverage/raw
    rm -f coverage/coverage.out coverage/coverage.txt coverage/index.html
    mkdir coverage/raw
fi

run_test() {
    if [[ $coverage != 1 ]]; then
        "$engine" run --rm --name "$container" "$@"
        return
    fi
    local status=0
    "$engine" run --name "$container" --env GOCOVERDIR=/coverage "$@" || status=$?
    "$engine" cp "$container:/coverage/." coverage/raw
    "$engine" rm "$container" >/dev/null
    return "$status"
}

for target in "${targets[@]}"; do
    tag="buildos-tests:${target//[^a-zA-Z0-9_.-]/-}"
    image=$target
    [[ $image == */* ]] || image="docker.io/library/$image"
    "$engine" build -f tests/Dockerfile --build-arg "TARGET_IMAGE=$image" --build-arg "COVERAGE=$coverage" -t "$tag" .
    for profile in tests/profile/*.yaml; do
        module=${profile##*/}
        module=${module%.yaml}
        [[ -z ${MODULE:-} || $MODULE == "$module" ]] || continue
        echo "=== $target: $module ==="
        run_test --env "BUILDOS_MODULE=$module" "$tag" -test.v -test.run '^TestModule$'
    done
    if [[ -z ${MODULE:-} ]]; then
        # List inside the image as well; the host needs no Go installation.
        tests=$("$engine" run --rm --name "$container" "$tag" -test.list '^Test')
        while IFS= read -r test; do
            [[ $test == Test* && $test != TestModule ]] || continue
            run_test "$tag" -test.v -test.run "^${test}$"
        done <<< "$tests"
    fi
done

if [[ $coverage == 1 ]]; then
    "$engine" build --target build -f tests/Dockerfile --build-arg COVERAGE=1 -t buildos-coverage .
    "$engine" run --rm -v "$PWD/coverage:/coverage:ro" --entrypoint go buildos-coverage \
        tool covdata textfmt -i=/coverage/raw -o=/dev/stdout > coverage/coverage.out
    "$engine" run --rm -v "$PWD/coverage:/coverage:ro" --entrypoint go buildos-coverage \
        tool cover -func=/coverage/coverage.out | tee coverage/coverage.txt
    "$engine" run --rm -v "$PWD/coverage:/coverage:ro" --entrypoint go buildos-coverage \
        tool cover -html=/coverage/coverage.out -o=/dev/stdout > coverage/index.html
    echo "Coverage report: $PWD/coverage/index.html"
fi
