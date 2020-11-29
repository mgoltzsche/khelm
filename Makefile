IMAGE ?= mgoltzsche/khelm

LDFLAGS ?= ''
USER := $(shell id -u)
PKG := github.com/mgoltzsche/khelm

BUILD_DIR = $(CURDIR)/build
GOSEC := $(BUILD_DIR)/bin/go-sec
GOMMIT := $(BUILD_DIR)/bin/gommit
GOLINT := $(BUILD_DIR)/bin/golint

REV := $(shell git rev-parse --short HEAD 2> /dev/null || echo 'unknown')
VERSION ?= $(shell echo "$$(git for-each-ref refs/tags/ --count=1 --sort=-version:refname --format='%(refname:short)' 2>/dev/null)-dev-$(REV)" | sed 's/^v//')
GO_LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) -s -w -extldflags '-static'
BUILDTAGS ?= 

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

khelm: $(BUILD_DIR)
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/bin/khelm -a -ldflags "$(GO_LDFLAGS)" -tags "$(BUILDTAGS)" ./cmd/khelm

install-kustomize-plugin:
	mkdir -p $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer
	cp $(BUILD_DIR)/bin/khelm $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer

image:
	docker build --force-rm -t $(IMAGE) .

test: $(BUILD_DIR)
	go test -coverprofile $(BUILD_DIR)/coverage.out -cover ./...

coverage: test
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html

e2e-test: image
	./e2e/image-test.sh
	./e2e/kpt-test.sh

fmt:
	go fmt ./...

clean:
	rm -rf $(BUILD_DIR)

check: gofmt vet golint gosec ## Runs all linters

gofmt:
	MSGS="$$(gofmt -s -d .)" && [ ! "$$MSGS" ] || (echo "$$MSGS"; false)

vet: ## Runs go vet
	go vet ./...

gosec: $(GOSEC) ## Runs gosec linter
	$(GOSEC) --quiet -exclude=G302,G304,G306 ./...

golint: $(GOLINT) ## Runs golint linter
	$(GOLINT) -set_exit_status ./...

$(GOSEC): $(BUILD_DIR) ## Installs gosec
	@{ \
	set -e ;\
	TMP_DIR=$$(mktemp -d) ;\
	cd $$TMP_DIR ;\
	GOPATH=$$TMP_DIR GO111MODULE=on go get github.com/securego/gosec/v2/cmd/gosec@v2.4.0 ;\
	cp $$TMP_DIR/bin/gosec $(GOSEC) ;\
	chmod -R u+w $$TMP_DIR ;\
	rm -rf $$TMP_DIR ;\
	}

$(GOLINT): $(BUILD_DIR) ## Installs golint
	@{ \
	set -e ;\
	TMP_DIR=$$(mktemp -d) ;\
	cd $$TMP_DIR ;\
	GOPATH=$$TMP_DIR GO111MODULE=on go get golang.org/x/lint/golint;\
	cp $$TMP_DIR/bin/golint $(GOLINT) ;\
	chmod -R u+w $$TMP_DIR ;\
	rm -rf $$TMP_DIR ;\
	}

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)/bin
