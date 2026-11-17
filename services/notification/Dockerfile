FROM debian:bookworm-slim

WORKDIR /root/

COPY services/notification/app-release .
COPY services/notification/config.yml ./config.yml

CMD ["./app-release"]
