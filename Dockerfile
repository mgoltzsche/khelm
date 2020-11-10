FROM golang:1.14-alpine3.12 AS build
RUN apk add --update --no-cache git
ENV GO111MODULE=on CGO_ENABLED=0
COPY go.mod go.sum /go/src/github.com/mgoltzsche/helmr/
WORKDIR /go/src/github.com/mgoltzsche/helmr
RUN go mod download
COPY main.go /go/src/github.com/mgoltzsche/helmr/
COPY pkg /go/src/github.com/mgoltzsche/helmr/pkg
RUN go build -o helmr -ldflags '-s -w -extldflags "-static"' . && mv helmr /usr/local/bin/

FROM alpine:3.12
COPY --from=build /usr/local/bin/helmr /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/helmr"]
