# Image for the Linux GUI smoke test (go/scripts/gui-smoke-linux.sh): Ubuntu 24.04 with Go,
# GTK 4 / WebKitGTK 6.0 headers, Xvfb, a window manager and X tools.
FROM ubuntu:24.04
COPY --from=golang:1.26 /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
	libgtk-4-dev libwebkitgtk-6.0-dev gcc pkg-config git ca-certificates curl xvfb dbus openbox \
	wmctrl xdotool x11-utils imagemagick fonts-dejavu-core procps dpkg-dev zip \
	&& rm -rf /var/lib/apt/lists/*
