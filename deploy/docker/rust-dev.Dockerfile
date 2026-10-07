# syntax=docker/dockerfile:1.7
#
# Hot-reload image for the Rust tracking service. Used only by the
# `make dev` flow via docker-compose.dev.yml — production still uses
# tracking/Dockerfile.
#
# The container expects:
#   - Source bind-mounted at /app
#   - $CARGO_HOME / $CARGO_TARGET_DIR on named volumes so the registry
#     and the compiled artifact cache survive container recreates AND
#     are shared across worktrees.
#
# cargo-watch handles the watch loop: on any change under /app/src,
# it re-runs `cargo run`. Incremental compile against the warm
# target/ dir typically takes 2-10s for a single-file change.

FROM rust:1.95-alpine

RUN apk add --no-cache \
    musl-dev cmake make gcc g++ pkgconfig \
    openssl-dev openssl-libs-static curl-dev zlib-static \
    git

# Pin cargo-watch so the dev image is reproducible.
RUN cargo install cargo-watch --version 8.5.3 --locked

ENV CARGO_HOME=/usr/local/cargo
ENV CARGO_TARGET_DIR=/app/target
ENV RUST_BACKTRACE=1

WORKDIR /app

# `cargo watch -x run` is the standard incremental dev loop. -q quiets
# the cargo-watch banner; the inner `cargo run` output still streams.
# We don't pass --release on purpose — debug builds are ~5x faster to
# compile, which is what dev iteration cares about.
ENTRYPOINT ["cargo", "watch", "-q", "-w", "src", "-w", "Cargo.toml", "-x", "run"]
