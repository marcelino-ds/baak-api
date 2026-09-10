FROM golang:1.24.1-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o baak-api .

FROM builder AS test
RUN apk add --no-cache gcc musl-dev
RUN CGO_ENABLED=1 go test -race -count=1 ./... && go vet ./...

# Final image with FlareSolverr + BAAK API
FROM ghcr.io/flaresolverr/flaresolverr:v3.5.0

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
user=root
logfile=/var/log/supervisor/supervisord.log
pidfile=/var/run/supervisord.pid

[program:flaresolverr]
command=/usr/local/bin/python -u /app/flaresolverr.py
directory=/app
priority=10
autostart=true
autorestart=true
user=flaresolverr
stopasgroup=true
killasgroup=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=PORT="8191",LOG_LEVEL="warning",TZ="Asia/Jakarta",HOME="/app"

[program:baak-api]
command=/usr/local/bin/baak-api
priority=20
autostart=true
autorestart=true
user=flaresolverr
stopwaitsecs=60
stopasgroup=true
killasgroup=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
EOF

ENV FLARESOLVERR_URL=http://127.0.0.1:8191 PORT=8080 CACHE_ENABLED=true

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import os, urllib.request; port = os.environ.get('PORT', '8080').strip().lstrip(':'); urllib.request.urlopen('http://127.0.0.1:' + port + '/live', timeout=3)" || exit 1

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
