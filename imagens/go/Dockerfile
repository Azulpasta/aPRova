FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS compilacao

ENV CGO_ENABLED=0 GOTOOLCHAIN=local

WORKDIR /fonte
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN go build -trimpath -ldflags="-s -w" -o /saida/aprova-sandbox ./cmd/sandbox

FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414

RUN apk add --no-cache git \
 && addgroup -g 10001 aprova \
 && adduser -D -H -u 10001 -G aprova -s /sbin/nologin aprova

COPY --from=compilacao /saida/aprova-sandbox /usr/local/bin/aprova-sandbox

ENTRYPOINT ["/usr/local/bin/aprova-sandbox"]
