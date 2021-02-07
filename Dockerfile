FROM golang:1.14-alpine3.12 AS build
RUN apk add --update --no-cache git
ENV GO111MODULE=on CGO_ENABLED=0
COPY go.mod go.sum /go/src/github.com/mgoltzsche/khelm/
WORKDIR /go/src/github.com/mgoltzsche/khelm
RUN go mod download
COPY cmd/khelm /go/src/github.com/mgoltzsche/khelm/cmd/khelm
COPY internal /go/src/github.com/mgoltzsche/khelm/internal
COPY pkg /go/src/github.com/mgoltzsche/khelm/pkg
ARG KHELM_VERSION=dev-build
ARG HELM_VERSION=unknown-version
RUN go build -o khelm -ldflags "-X main.khelmVersion=$KHELM_VERSION -X main.helmVersion=$HELM_VERSION -s -w -extldflags '-static'" ./cmd/khelm && mv khelm /usr/local/bin/

FROM alpine:3.12
RUN mkdir /helm && chown root:nobody /helm && chmod 1777 /helm
ENV HELM_HOME=/helm
COPY --from=build /usr/local/bin/khelm /usr/local/bin/khelmfn
ENTRYPOINT ["/usr/local/bin/khelmfn"]
