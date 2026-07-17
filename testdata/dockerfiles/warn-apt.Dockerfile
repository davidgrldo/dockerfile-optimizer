FROM debian:bookworm
RUN apt-get update && apt-get install -y curl git
ADD https://example.com/tool.tar.gz /opt/tool.tar.gz
COPY entrypoint.sh /usr/local/bin/entrypoint
USER root
ENTRYPOINT ["/usr/local/bin/entrypoint"]
