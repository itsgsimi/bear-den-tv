# Headless sway for the Wayland adapter integration test
# (scripts/wayland-container-test.sh, docs/decisions/0007-wayland-profile.md).
# Only the compositor and small test clients live here; the coordinator binary
# is built on the host and mounted read-only.
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends sway foot xwayland x11-utils dbus ca-certificates \
 && rm -rf /var/lib/apt/lists/*
