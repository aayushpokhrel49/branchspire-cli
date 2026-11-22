FROM docker.io/chainguard/go:latest@sha256:78352255a8760797dac56865a5b9af2881f245ef74c90199e4e15f52251c5357 AS build
COPY . /build/
WORKDIR /build
RUN make build && mkdir -p /app/.config/branchspire-cli

FROM docker.io/chainguard/busybox:latest-glibc@sha256:63f6bae487adff606359bd82418ffe0564564a6b272cf52107306749a714afb2
COPY --from=build /build/bh /bin/bh
COPY --from=build --chown=65532:65532 /app /app
VOLUME [ "/app" ]
ENV HOME="/app"
ENTRYPOINT ["/bin/bh"]
