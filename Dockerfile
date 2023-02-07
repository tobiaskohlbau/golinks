FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

FROM scratch
LABEL org.opencontainers.image.source https://github.com/tobiaskohlbau/golinks
COPY golinks /golinks
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/golinks"]