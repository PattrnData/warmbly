# syntax=docker/dockerfile:1.7
#
# Hot-reload image for the Elixir/Phoenix realtime service. Used only
# by the `make dev` flow — production still uses realtime.Dockerfile
# with `mix release`.
#
# Phoenix already has built-in code reloading via the BEAM, so we
# don't need an external file watcher. `mix phx.server` boots once and
# the Phoenix endpoint hot-recompiles modules whenever source files
# change under bind-mounted /app.
#
# The container expects:
#   - realtime/ bind-mounted at /app
#   - deps/ and _build/ on named volumes so deps survive container
#     recreates AND are shared across worktrees.

FROM elixir:1.18-otp-27-alpine

RUN apk add --no-cache git build-base inotify-tools

RUN mix local.hex --force && mix local.rebar --force

ENV MIX_ENV=dev
ENV ERL_AFLAGS="-kernel shell_history enabled"

WORKDIR /app

# Wrapper script: deps.get is cheap when the lock matches deps/, so
# running it on every boot is the right tradeoff — it picks up
# mix.lock changes without forcing the operator to rebuild the image.
COPY deploy/docker/elixir-dev-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
