FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS compilacao

ENV CGO_ENABLED=0 GOTOOLCHAIN=local

WORKDIR /fonte
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN go build -trimpath -ldflags="-s -w" -o /saida/aprova-sandbox ./cmd/sandbox

FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d

RUN apk add --no-cache git \
 && addgroup -g 10001 aprova \
 && adduser -D -H -u 10001 -G aprova -s /sbin/nologin aprova

COPY --from=compilacao /saida/aprova-sandbox /usr/local/bin/aprova-sandbox

ENTRYPOINT ["/usr/local/bin/aprova-sandbox"]
