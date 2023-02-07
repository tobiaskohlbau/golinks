FROM scratch
LABEL org.opencontainers.image.source https://github.com/tobiaskohlbau/golinks
COPY golinks /golinks
ENTRYPOINT ["/golinks"]