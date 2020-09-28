LDFLAGS?=''
USER=$(shell id -u)
PKG=github.com/mgoltzsche/helm-kustomize-plugin

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
	cp helm-kustomize-plugin $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer

test:
	go test -coverprofile coverage.out -cover ./...

coverage: test
	go tool cover -html=coverage.out -o coverage.html

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

ide:
	mkdir -p .build-cache
	docker run -d --name liteide-helm-kustomize-plugin --rm \
		-e DISPLAY="$(shell echo $$DISPLAY)" \
		-e CHUSR=$(shell id -u):$(shell id -g) \
		-e GO111MODULE=on \
		-e GOFLAGS=' ' \
		--mount type=bind,src=/tmp/.X11-unix,dst=/tmp/.X11-unix \
		--mount type=bind,src=/etc/machine-id,dst=/etc/machine-id \
		--mount "type=bind,src=$(shell pwd)/.build-cache,dst=/go" \
		--mount "type=bind,src=$(shell pwd),dst=/go/src/$(PKG)" \
		"$(LITEIDEIMAGE)" \
		"/go/src/$(PKG)"
