FROM golang:1.16.2-alpine3.13 AS build
MAINTAINER rthellend@gmail.com
RUN apk update && apk upgrade
RUN apk add --no-cache libsodium-dev=1.0.18-r0 gcc=10.2.1_pre1-r3 musl-dev=1.2.2-r0

ADD c2FmZQ/go.mod /app/go/src/c2FmZQ/go.mod
ADD c2FmZQ/go.sum /app/go/src/c2FmZQ/go.sum
WORKDIR /app/go/src/c2FmZQ
RUN go mod download

ADD c2FmZQ /app/go/src/c2FmZQ
RUN go install ./c2FmZQ-server
RUN go install ./c2FmZQ-server/inspect

FROM alpine:3.13
RUN apk update && apk upgrade
RUN apk add --no-cache libsodium=1.0.18-r0
RUN mkdir -p /app/bin
COPY --from=build /go/bin/c2FmZQ-server /go/bin/inspect /app/bin/
WORKDIR /app

EXPOSE 80 443
VOLUME ["/data", "/secrets"]

ENV C2FMZQ_PASSPHRASE_FILE=/secrets/passphrase
ENV C2FMZQ_DATABASE=/data
ENV PATH=/app/bin:$PATH

ENTRYPOINT ["/app/bin/c2FmZQ-server"]
# For HTTPS
CMD ["-address=:443", "-tlskey=/secrets/privkey.pem", "-tlscert=/secrets/fullchain.pem"]
# For HTTP
#CMD ["-address=:80"]
