FROM gcr.io/distroless/static

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/torrents /usr/bin/torrents

ENTRYPOINT ["/usr/bin/torrents"]
CMD ["serve"]
