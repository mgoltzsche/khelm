IMAGE ?= mgoltzsche/khelm:latest

BUILD_DIR := $(CURDIR)/build
KHELM := $(BUILD_DIR)/bin/khelm
KHELM_STATIC := $(BUILD_DIR)/bin/khelm-static
GOSEC := $(BUILD_DIR)/bin/go-sec
GOLINT := $(BUILD_DIR)/bin/golint
KPT := $(BUILD_DIR)/bin/kpt
KUSTOMIZE := $(BUILD_DIR)/bin/kustomize

KPT_VERSION ?= 0.37.1
KUSTOMIZE_VERSION ?= 3.9.3

REV := $(shell git rev-parse --short HEAD 2> /dev/null || echo 'unknown')
VERSION ?= $(shell echo "$$(git describe --exact-match --tags $(git log -n1 --pretty='%h') 2> /dev/null || echo dev)-$(REV)" | sed 's/^v//')
HELM_VERSION := $(shell grep helm\.sh/helm/ go.mod | sed -E -e 's/helm\.sh\/helm\/v3|\s+//g' -e 's/^v//')
GO_LDFLAGS := -X main.khelmVersion=$(VERSION) -X main.helmVersion=$(HELM_VERSION)
BUILDTAGS ?= 

all: clean khelm test check

khelm: $(KHELM)

khelm-static: $(KHELM_STATIC)

$(KHELM): $(BUILD_DIR)
	go build -o $(BUILD_DIR)/bin/khelm -a -ldflags "$(GO_LDFLAGS)" -tags "$(BUILDTAGS)" ./cmd/khelm

$(KHELM_STATIC): image $(BUILD_DIR)
	@echo Copying khelm binary from container
	@{ \
	set -e; \
	CONTAINER=`docker create $(IMAGE)`; \
	docker cp $$CONTAINER:/usr/local/bin/khelmfn $(KHELM_STATIC); \
	[ -f $(KHELM) ] || cp $(KHELM_STATIC) $(KHELM); \
	docker rm -f $$CONTAINER >/dev/null; \
	}

install: khelm
	cp $(BUILD_DIR)/bin/khelm /usr/local/bin/khelm
	chmod +x /usr/local/bin/khelm

install-kustomize-plugin:
	mkdir -p $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer
	cp $(BUILD_DIR)/bin/khelm $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer

image:
	docker build --force-rm -t $(IMAGE) --build-arg KHELM_VERSION=$(VERSION) --build-arg HELM_VERSION=$(HELM_VERSION) .

test: $(BUILD_DIR)
	go test -coverprofile $(BUILD_DIR)/coverage.out -cover ./...

coverage: test
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html

e2e-test: image khelm-static kpt kustomize
	@echo
	@echo 'RUNNING E2E TESTS (PATH=$(BUILD_DIR)/bin)...'
	@{ \
	set -e ; \
	export PATH="$(BUILD_DIR)/bin:$$PATH"; \
	./e2e/kustomize-plugin-test.sh; \
	IMAGE=$(IMAGE) ./e2e/image-cli-test.sh; \
	./e2e/kpt-function-test.sh; \
	./e2e/kpt-cert-manager-test.sh; \
	./e2e/kpt-linkerd-test.sh; \
	}

fmt:
	go fmt ./...

clean:
	rm -f $(BUILD_DIR)/bin/khelm
	rm -f $(BUILD_DIR)/bin/khelm-static

clean-all:
	rm -rf $(BUILD_DIR)
	find . -name charts -type d -exec rm -rf {} \;

check: gofmt vet golint gosec ## Runs all linters

gofmt:
	MSGS="$$(gofmt -s -d .)" && [ ! "$$MSGS" ] || (echo "$$MSGS" >&2; echo 'Please run `make fmt` to fix it' >&2; false)

vet:
	go vet ./...

gosec: $(GOSEC)
	$(GOSEC) --quiet -exclude=G302,G304,G306 ./...

golint: $(GOLINT)
	$(GOLINT) -set_exit_status ./...

kpt: $(KPT)

kustomize: $(KUSTOMIZE)

$(GOSEC): $(BUILD_DIR)
	@echo Building gosec
	@{ \
	set -e; \
	TMP_DIR=$$(mktemp -d); \
	(cd $$TMP_DIR && \
	GOPATH=$$TMP_DIR GO111MODULE=on go get github.com/securego/gosec/v2/cmd/gosec@v2.4.0); \
	cp $$TMP_DIR/bin/gosec $(GOSEC); \
	chmod -R u+w $$TMP_DIR; \
	rm -rf $$TMP_DIR; \
	}

$(GOLINT): $(BUILD_DIR)
	@echo Building golint
	@{ \
	set -e; \
	TMP_DIR=$$(mktemp -d); \
	(cd $$TMP_DIR && \
	GOPATH=$$TMP_DIR GO111MODULE=on go get golang.org/x/lint/golint); \
	cp $$TMP_DIR/bin/golint $(GOLINT); \
	chmod -R u+w $$TMP_DIR; \
	rm -rf $$TMP_DIR; \
	}

$(KPT): $(BUILD_DIR)
	@echo Downloading kpt
	@{ \
	set -e; \
	TMP_DIR=$$(mktemp -d); \
	curl -fsSL https://github.com/GoogleContainerTools/kpt/releases/download/v$(KPT_VERSION)/kpt_linux_amd64-$(KPT_VERSION).tar.gz | tar -xzf - -C $$TMP_DIR; \
	cp -f $$TMP_DIR/kpt $(KPT); \
	chmod -R +x $(KPT); \
	rm -rf $$TMP_DIR; \
	}

$(KUSTOMIZE): $(BUILD_DIR)
	@echo Downloading kustomize
	@{ \
	set -e; \
	TMP_DIR=$$(mktemp -d); \
	curl -fsSL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv$(KUSTOMIZE_VERSION)/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz | tar -xzf - -C $$TMP_DIR; \
	cp -f $$TMP_DIR/kustomize $(KUSTOMIZE); \
	chmod -R +x $(KUSTOMIZE); \
	rm -rf $$TMP_DIR; \
	}

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)/bin
