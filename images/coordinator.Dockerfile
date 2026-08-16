FROM docker.io/library/golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36

ARG OPENCODE_VERSION=1.18.18
ARG OPENCODE_SHA256=0cddc222418b8553669905a8980c0cda7088f00da24d83d6ac76b01c9fdb2aaf

ADD --checksum=sha256:0cddc222418b8553669905a8980c0cda7088f00da24d83d6ac76b01c9fdb2aaf \
    https://github.com/anomalyco/opencode/releases/download/v1.18.18/opencode-linux-x64.tar.gz \
    /tmp/opencode.tar.gz

ADD --checksum=sha256:193e3a33dc66d679d522c40e6a5138788646d3c6a5039d0c5c2b3bede83b6635 \
    https://registry.npmjs.org/zod/-/zod-4.1.8.tgz \
    /tmp/zod.tar.gz

RUN tar -xzf /tmp/opencode.tar.gz -C /usr/local/bin \
    && mkdir -p /opt/harness/node_modules/zod /opt/readonly-config/opencode \
    && tar -xzf /tmp/zod.tar.gz -C /opt/harness/node_modules/zod --strip-components=1 \
    && rm /tmp/opencode.tar.gz /tmp/zod.tar.gz \
    && test "$(opencode --version)" = "${OPENCODE_VERSION}" \
    && useradd --create-home --uid 10001 --shell /bin/bash coordinator \
    && touch /opt/readonly-config/opencode/.gitignore /opt/readonly-config/.gitignore \
    && chmod -R 0555 /opt/readonly-config

COPY harness/tool-bridge.ts /opt/harness/tool-bridge.ts

ENV OPENCODE_DISABLE_AUTOUPDATE=1 \
    OPENCODE_DISABLE_DEFAULT_PLUGINS=1 \
    OPENCODE_DISABLE_EXTERNAL_SKILLS=1 \
    OPENCODE_DISABLE_CLAUDE_CODE=1 \
    OPENCODE_DISABLE_LSP_DOWNLOAD=1 \
    OPENCODE_CONFIG_DIR=/opt/readonly-config \
    XDG_CONFIG_HOME=/opt/readonly-config

USER coordinator
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/opencode"]
