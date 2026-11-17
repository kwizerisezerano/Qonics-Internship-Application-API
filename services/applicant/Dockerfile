FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

COPY services/applicant/app-release .
COPY services/applicant/config.yml.example ./config.yml

EXPOSE 9000

CMD ["./app-release"]
