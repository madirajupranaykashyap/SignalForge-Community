
FROM golang:1.27.3 AS builder

WORKDIR /src

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.159.0

COPY . .

RUN builder \
    --config=collector/builder-config.yaml \
    --output-path=/out

FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S signalforge \
    && adduser -S -G signalforge signalforge \
    && mkdir -p /var/lib/signalforge/archive /etc/signalforge \
    && chown -R signalforge:signalforge /var/lib/signalforge

COPY --from=builder /out/signalforge-collector /usr/local/bin/signalforge-collector

# Include the Collector configuration in the image.
COPY collector/config/signalforge-collector.yaml /etc/signalforge/collector.yml

USER signalforge

EXPOSE 4317 4318

ENTRYPOINT ["/usr/local/bin/signalforge-collector"]
CMD ["--config=/etc/signalforge/collector.yml"]
