FROM golang:1.26-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/main .

FROM debian:trixie-slim
ARG TARGETARCH=amd64
ARG COREDNS_VERSION=1.14.7

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl iproute2 iptables wireguard-tools \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir /app \
 && cd /tmp \
 && curl -fsSLO "https://github.com/coredns/coredns/releases/download/v${COREDNS_VERSION}/coredns_${COREDNS_VERSION}_linux_${TARGETARCH}.tgz" \
 && curl -fsSLO "https://github.com/coredns/coredns/releases/download/v${COREDNS_VERSION}/coredns_${COREDNS_VERSION}_linux_${TARGETARCH}.tgz.sha256" \
 && sha256sum -c "coredns_${COREDNS_VERSION}_linux_${TARGETARCH}.tgz.sha256" \
 && tar xzf "coredns_${COREDNS_VERSION}_linux_${TARGETARCH}.tgz" -C /app \
 && rm -rf /tmp/*

WORKDIR /usr/src/wireguard-manager-and-api
COPY Corefile /app/Corefile
COPY services/start.sh services/start.sh
COPY --from=build /out/main main

EXPOSE 8443 51820/udp
CMD ["./services/start.sh"]
