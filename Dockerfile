FROM gcr.io/distroless/static

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/torrents /usr/bin/torrents
COPY LICENSE.md /usr/bin/LICENSE.md
COPY README.md /usr/bin/README.md
COPY licenses /usr/bin/licenses

ENTRYPOINT ["/usr/bin/torrents"]
CMD ["serve"]
