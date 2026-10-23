FROM golang:1.27.3 AS builder

WORKDIR /src

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.159.0

COPY . .
RUN go mod download

RUN builder \
    --config=collector/builder-config.yaml \
    --output-path=/out

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/signalforge-agent ./cmd/collector-agent

FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S signalforge \
    && adduser -S -G signalforge signalforge \
    && mkdir -p /var/lib/signalforge/archive /var/lib/signalforge/config /etc/signalforge \
    && chown -R signalforge:signalforge /var/lib/signalforge

COPY --from=builder /out/signalforge-collector /usr/local/bin/signalforge-collector
COPY --from=builder /out/signalforge-agent /usr/local/bin/signalforge-agent
COPY collector/config/signalforge-collector.yaml /etc/signalforge/collector.yml

ENV SIGNALFORGE_DROP_SEVERITY=DEBUG \
    SIGNALFORGE_HEALTH_ROUTE=/health \
    SIGNALFORGE_HEALTH_SERVICE=health-check

USER signalforge

EXPOSE 4317 4318 9464

ENTRYPOINT ["/usr/local/bin/signalforge-agent"]
