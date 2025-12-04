FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o baak-api .

# Final image with FlareSolverr + BAAK API
FROM ghcr.io/flaresolverr/flaresolverr:latest

# Install supervisor to run multiple processes
USER root
RUN apt-get update && apt-get install -y supervisor && rm -rf /var/lib/apt/lists/*

# Copy BAAK API binary
COPY --from=builder /app/baak-api /usr/local/bin/baak-api

# Create supervisor config
RUN mkdir -p /var/log/supervisor
COPY <<EOF /etc/supervisor/conf.d/supervisord.conf
[supervisord]
nodaemon=true
logfile=/var/log/supervisor/supervisord.log
pidfile=/var/run/supervisord.pid

[program:flaresolverr]
command=/usr/local/bin/flaresolverr
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=LOG_LEVEL="info",TZ="Asia/Jakarta"

[program:baak-api]
command=/usr/local/bin/baak-api
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=FLARESOLVERR_URL="http://localhost:8191",CACHE_ENABLED="true"
EOF

# Expose ports
EXPOSE 8080 8191

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
