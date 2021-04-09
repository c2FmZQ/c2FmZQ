FROM golang:1.16.2-alpine3.13 AS build
MAINTAINER rthellend@gmail.com
RUN apk update && apk upgrade
RUN apk add --no-cache libsodium-dev=1.0.18-r0 gcc=10.2.1_pre1-r3 musl-dev=1.2.2-r0

ADD kringle-server/go.mod /app/go/src/kringle-server/go.mod
ADD kringle-server/go.sum /app/go/src/kringle-server/go.sum
WORKDIR /app/go/src/kringle-server
RUN go mod download

ADD kringle-server /app/go/src/kringle-server
RUN go install

FROM alpine:3.13
RUN apk update && apk upgrade
RUN apk add --no-cache libsodium=1.0.18-r0
RUN mkdir -p /app/bin
COPY --from=build /go/bin/kringle-server /app/bin
WORKDIR /app

EXPOSE 80 443
VOLUME ["/data", "/secrets"]

ENTRYPOINT ["/app/bin/kringle-server", "-db=/data"]
# For HTTPS
CMD ["-address=:443", "-passphrase-file=/secrets/kringle-passphrase", "-tlskey=/secrets/privkey.pem", "-tlscert=/secrets/fullchain.pem"]
# For HTTP
#CMD ["-address=:80", "-passphrase-file=/secrets/kringle-passphrase"]
