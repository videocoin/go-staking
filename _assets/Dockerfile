FROM golang:1.13-alpine as builder

RUN apk add --no-cache make gcc musl-dev linux-headers
WORKDIR /build/
ADD . .

RUN make build

FROM alpine:latest

COPY --from=builder /build/build/setter /

ENTRYPOINT ["/setter"]