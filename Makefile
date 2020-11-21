IMAGE?=mgoltzsche/helmr

LDFLAGS?=''
USER=$(shell id -u)
PKG=github.com/mgoltzsche/helmr

BUILDTAGS?=

GOIMAGE=helm-kustomize-plugin-go
LITEIDEIMAGE=mgoltzsche/liteide:x36
DOCKERRUN=docker run --name helm-kustomize-plugin-build --rm \
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

all: clean build

build: golang-image
	$(DOCKERRUN) $(GOIMAGE) \
		make helm-kustomize-plugin BUILDTAGS=$(BUILDTAGS)

helm-kustomize-plugin:
	CGO_ENABLED=0 go build -a -ldflags '-s -w -extldflags "-static" $(LDFLAGS)' -tags '$(BUILDTAGS)' .

install:
	mkdir -p $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer
	cp helmr $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer

test:
	go test -coverprofile coverage.out -cover ./...

coverage: test
	go tool cover -html=coverage.out -o coverage.html

e2e-test: image
	# TODO: make sure repositories.yaml is not created when it doesn't exist
	#./e2e/image-test.sh
	./e2e/kpt-test.sh

clean:
	rm -f helm-kustomize-plugin coverage.out coverage.html

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
		make clean helm-kustomize-plugin test lint check-fmt BUILDTAGS=$(BUILDTAGS)

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
