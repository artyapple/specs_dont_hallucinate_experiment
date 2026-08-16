FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS generator-builder

RUN GOBIN=/out go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
    && GOBIN=/out go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 \
    && test "$(/out/oapi-codegen --version | tail -n 1)" = "v2.8.0" \
    && test "$(/out/sqlc version)" = "v1.31.1"

FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS bridge-builder

COPY harness/bridge /src
RUN cd /src && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -o /out/tool-bridge .

FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS tool-base

ARG OPENCODE_VERSION=1.18.18

ADD --checksum=sha256:0cddc222418b8553669905a8980c0cda7088f00da24d83d6ac76b01c9fdb2aaf \
    https://github.com/anomalyco/opencode/releases/download/v1.18.18/opencode-linux-x64.tar.gz \
    /tmp/opencode.tar.gz

RUN tar -xzf /tmp/opencode.tar.gz -C /usr/local/bin \
    && rm /tmp/opencode.tar.gz \
    && test "$(opencode --version)" = "${OPENCODE_VERSION}" \
    && useradd --create-home --uid 10001 --shell /bin/bash candidate \
    && mkdir -p /workspace /home/candidate/.cache/go-build \
    && chown -R candidate:candidate /workspace /home/candidate

COPY --from=bridge-builder /out/tool-bridge /usr/local/bin/tool-bridge

ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOSUMDB=off \
    GOCACHE=/home/candidate/.cache/go-build \
    HOME=/home/candidate

USER candidate
WORKDIR /workspace

EXPOSE 4096
CMD ["/usr/local/bin/tool-bridge"]

FROM tool-base AS direct

FROM tool-base AS codegen

COPY --from=generator-builder /out/oapi-codegen /usr/local/bin/oapi-codegen
COPY --from=generator-builder /out/sqlc /usr/local/bin/sqlc
