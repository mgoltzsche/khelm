IMAGE?=mgoltzsche/khelm

LDFLAGS?=''
USER=$(shell id -u)
PKG=github.com/mgoltzsche/khelm

BUILDTAGS?=

GOIMAGE=khelm-go
DOCKERRUN=docker run --rm \
		-v "$(shell pwd):/go/src/$(PKG)" \
		-w "/go/src/$(PKG)" \
		-u $(USER):$(USER) \
		-e HOME=/go \
		-e CGO_ENABLED=0
define GODOCKERFILE
FROM golang:1.14-alpine3.12
RUN apk add --update --no-cache make git
RUN go get golang.org/x/lint/golint
endef
export GODOCKERFILE

all: clean khelm-docker

khelm-docker: golang-image
	$(DOCKERRUN) $(GOIMAGE) \
		make khelm BUILDTAGS=$(BUILDTAGS)

khelm: builddir
	CGO_ENABLED=0 go build -o build/bin/khelm -a -ldflags '-s -w -extldflags "-static" $(LDFLAGS)' -tags '$(BUILDTAGS)' ./cmd/khelm

install-kustomize-plugin:
	mkdir -p $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer
	cp build/bin/khelm $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer

test: builddir
	go test -coverprofile build/coverage.out -cover ./...

coverage: test
	go tool cover -html=build/coverage.out -o build/coverage.html

e2e-test: image
	./e2e/image-test.sh
	./e2e/kpt-test.sh

clean:
	rm -rf build

check-fmt-docker: golang-image
	$(DOCKERRUN) $(GOIMAGE) make check-fmt
check-fmt:
	cd "$$GOPATH/src" && MSGS="$$(gofmt -s -d $(shell go list ./pkg/...))" && [ ! "$$MSGS" ] || (echo "$$MSGS"; false)

lint-docker: golang-image
	$(DOCKERRUN) $(GOIMAGE) make lint
lint:
	golint -set_exit_status $(shell go list ./...)

check: golang-image
	$(DOCKERRUN) $(GOIMAGE) \
		make clean khelm test lint check-fmt BUILDTAGS=$(BUILDTAGS)

coverage-report: golang-image
	$(DOCKERRUN) $(GOIMAGE) make coverage
	firefox coverage.html

vendor-update: golang-image
	mkdir -p .build-cache
	$(DOCKERRUN) -e GO111MODULE=on \
		--mount "type=bind,src=$(shell pwd)/.build-cache,dst=/go" \
		$(GOIMAGE) go mod vendor

golang-image:
	echo "$$GODOCKERFILE" | docker build --force-rm -t $(GOIMAGE) -

image:
	docker build --force-rm -t $(IMAGE) .

builddir:
	mkdir -p build/bin
