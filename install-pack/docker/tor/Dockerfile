FROM alpine:3.21

# su-exec is small and perfect for dropping privileges after root preparation
RUN apk add --no-cache tor su-exec

COPY torrc /etc/tor/torrc
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
