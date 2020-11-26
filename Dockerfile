FROM golang:1.14-alpine3.12 AS build
RUN apk add --update --no-cache git
ENV GO111MODULE=on CGO_ENABLED=0
COPY go.mod go.sum /go/src/github.com/mgoltzsche/khelm/
WORKDIR /go/src/github.com/mgoltzsche/khelm
RUN go mod download
COPY cmd/khelm /go/src/github.com/mgoltzsche/khelm/cmd/khelm
COPY pkg /go/src/github.com/mgoltzsche/khelm/pkg
RUN go build -o build/bin/khelm -ldflags '-s -w -extldflags "-static"' ./cmd/khelm && mv build/bin/khelm /usr/local/bin/

FROM alpine:3.12
RUN mkdir /helm && chown root:nobody /helm && chmod 775 /helm
ENV HELM_HOME=/helm
COPY --from=build /usr/local/bin/khelm /usr/local/bin/khelmfn
ENTRYPOINT ["/usr/local/bin/khelmfn"]
