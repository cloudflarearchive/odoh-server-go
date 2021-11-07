FROM alpine
COPY odohd /usr/bin/odohd
ENTRYPOINT ["/usr/bin/odohd"]
