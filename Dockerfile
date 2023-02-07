FROM ubuntu:22.04
RUN update-ca-certificates

FROM scratch
LABEL org.opencontainers.image.source https://github.com/tobiaskohlbau/golinks
COPY golinks /golinks
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/golinks"]