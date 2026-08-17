# Hidden evaluator image. Builds the evaluator binary from pinned sources and
# preloads the exact fixture dependency graph so candidate builds never need
# network access. The image contains no provider credentials and must never
# receive any at runtime.

FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS evaluator-builder

WORKDIR /src
COPY evaluator/go.mod evaluator/go.sum ./
RUN go mod download
COPY evaluator/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -buildvcs=false -o /out/evaluator ./cmd/evaluator \
    && test -x /out/evaluator

# Preload the union of fixture module dependencies. Candidates derive from the
# fixtures, and the frozen network policy forbids package downloads, so the
# module cache must satisfy every candidate build offline.
FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS module-cache

WORKDIR /preload
COPY fixtures/base1/go.mod fixtures/base1/go.sum ./base1/
COPY fixtures/base2-direct/go.mod fixtures/base2-direct/go.sum ./base2-direct/
COPY fixtures/base2-codegen/go.mod fixtures/base2-codegen/go.sum ./base2-codegen/
COPY fixtures/task-solutions/nullable-patch-codegen/go.mod fixtures/task-solutions/nullable-patch-codegen/go.sum ./nullable-patch-codegen/
RUN for module in base1 base2-direct base2-codegen nullable-patch-codegen; do \
      (cd "/preload/$module" && go mod download); \
    done

FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36

COPY --from=evaluator-builder /out/evaluator /usr/local/bin/evaluator
COPY --from=module-cache /go/pkg/mod /go/pkg/mod

RUN useradd --create-home --uid 10001 --shell /bin/bash evaluator \
    && mkdir -p /home/evaluator/.cache/go-build /candidate \
    && chown -R evaluator:evaluator /home/evaluator /candidate

ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOSUMDB=off \
    GOCACHE=/home/evaluator/.cache/go-build \
    GOPATH=/go \
    HOME=/home/evaluator

USER evaluator
WORKDIR /candidate

ENTRYPOINT ["/usr/local/bin/evaluator"]
