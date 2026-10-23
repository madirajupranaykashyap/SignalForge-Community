FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/signalforge-management ./cmd/management

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S signalforge \
    && adduser -S -G signalforge signalforge \
    && mkdir -p /var/lib/signalforge \
    && chown -R signalforge:signalforge /var/lib/signalforge
COPY --from=build /out/signalforge-management /usr/local/bin/signalforge-management
ENV SIGNALFORGE_MANAGEMENT_ADDR=0.0.0.0:8081 \
    SIGNALFORGE_MANAGEMENT_DB=/var/lib/signalforge/management.db
USER signalforge
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/signalforge-management"]
