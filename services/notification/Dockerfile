FROM debian:bookworm-slim

# Install CA certificates so smtp.SendMail can verify the SMTP server's TLS
# certificate (required for STARTTLS, e.g. smtp.gmail.com:587).
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

COPY services/notification/app-release .
COPY services/notification/config.yml ./config.yml

# HTML email templates (leazi pattern) are resolved from /app/templates.
COPY services/notification/templates /app/templates

CMD ["./app-release"]
