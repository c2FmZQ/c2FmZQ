FROM golang:1.18.3-alpine3.16 AS build
MAINTAINER rthellend@gmail.com
RUN apk update && apk upgrade

ADD c2FmZQ/go.mod /app/go/src/c2FmZQ/go.mod
ADD c2FmZQ/go.sum /app/go/src/c2FmZQ/go.sum
WORKDIR /app/go/src/c2FmZQ
RUN go mod download

ADD c2FmZQ /app/go/src/c2FmZQ
RUN CGO_ENABLED=0 go test ./internal/server/...
RUN go install ./c2FmZQ-server
RUN go install ./c2FmZQ-server/inspect

FROM alpine:3.16
RUN apk update && apk upgrade
RUN mkdir -p /app/bin
COPY --from=build /go/bin/c2FmZQ-server /go/bin/inspect /app/bin/
WORKDIR /app

EXPOSE 80 443
VOLUME ["/data", "/secrets"]

ENV C2FMZQ_PASSPHRASE_FILE=/secrets/passphrase
ENV C2FMZQ_DATABASE=/data
ENV PATH=/app/bin:$PATH

ENTRYPOINT ["/app/bin/c2FmZQ-server"]
# For HTTPS with TLS certs fetched directly from letencrypt.org. Pass the
# domain in env variable DOMAIN.
CMD ["-address=:443", "--redirect-404=https://c2FmZQ.org/"]
# For HTTP:
#CMD ["-address=:80"]
# For HTTPS with existing key and certs:
#CMD ["-address=:443", "-tlskey=/secrets/privkey.pem", "-tlscert=/secrets/fullchain.pem"]

