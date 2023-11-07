FROM golang:1.21.4-alpine3.18 AS build
MAINTAINER info@c2fmzq.org
RUN apk update && apk upgrade

ADD c2FmZQ/go.mod /app/go/src/c2FmZQ/go.mod
ADD c2FmZQ/go.sum /app/go/src/c2FmZQ/go.sum
WORKDIR /app/go/src/c2FmZQ
RUN go mod download

ADD c2FmZQ /app/go/src/c2FmZQ
RUN CGO_ENABLED=0 go test ./internal/server/...
RUN go install ./c2FmZQ-server
RUN go install ./c2FmZQ-server/inspect

FROM alpine:3.18
RUN apk update && apk upgrade
RUN apk add ca-certificates
RUN mkdir -p /app/bin
COPY --from=build /go/bin/c2FmZQ-server /go/bin/inspect /app/bin/
WORKDIR /app

EXPOSE 80 443
VOLUME ["/data", "/secrets"]

ENV PATH=/app/bin:$PATH

#################################
# Environment setting variables #

# For HTTPS set to ":443", for HTTP set to ":80"
ENV C2FMZQ_ADDRESS=":443"
#ENV C2FMZQ_ALLOW_NEW_ACCOUNTS
#ENV C2FMZQ_AUTO_APPROVE_NEW_ACCOUNTS
#ENV C2FMZQ_AUTOCERT_ADDRESS
#ENV C2FMZQ_BASE_URL
ENV C2FMZQ_DATABASE=/data
# To fetch TLS certs directly from letencrypt.org:
#ENV C2FMZQ_DOMAIN
#ENV C2FMZQ_ENABLE_WEBAPP
#ENV C2FMZQ_ENCRYPT_METADATA
#ENV C2FMZQ_HTDIGEST_FILE
#ENV C2FMZQ_MAX_CONCURRENT_REQUESTS
#ENV C2FMZQ_PASSPHRASE
#ENV C2FMZQ_PASSPHRASE_CMD
ENV C2FMZQ_PASSPHRASE_FILE=/secrets/passphrase
#ENV C2FMZQ_PATH_PREFIX
ENV C2FMZQ_REDIRECT_404="https://c2FmZQ.org/"
# For existing tls/https cert, e.g. "/secrets/privkey.pem"
#ENV C2FMZQ_TLSCERT
# For existing tls/https key, e.g. "/secrets/fullchain.pem"
#ENV C2FMZQ_TLSKEY
#ENV C2FMZQ_VERBOSE

#################################

ENTRYPOINT ["/app/bin/c2FmZQ-server"]
CMD []
