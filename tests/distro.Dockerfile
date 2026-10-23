ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ENV container=docker
RUN if command -v apt-get >/dev/null; then \
      apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y systemd systemd-sysv curl ca-certificates util-linux procps; \
    else dnf -y install systemd ca-certificates util-linux procps-ng && \
      { command -v curl >/dev/null || dnf -y install curl; }; fi
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
