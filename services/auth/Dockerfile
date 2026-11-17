FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

COPY services/auth/app-release .
COPY services/auth/config.yml.example ./config.yml

EXPOSE 9000

CMD ["./app-release"]
