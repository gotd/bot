FROM ghcr.io/go-faster/ubuntu:20.04

ADD bot /usr/local/bin/bot

ENTRYPOINT ["bot"]
