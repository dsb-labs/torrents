FROM gcr.io/distroless/static

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/torrents /usr/bin/torrents
COPY licenses /usr/bin/licenses

ENTRYPOINT ["/usr/bin/torrents"]
CMD ["serve"]
