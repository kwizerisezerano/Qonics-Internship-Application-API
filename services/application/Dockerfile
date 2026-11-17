FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

RUN mkdir -p /root/uploads/applications

COPY services/application/app-release .
COPY services/application/config.yml.example ./config.yml

EXPOSE 9000

CMD ["./app-release"]
