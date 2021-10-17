IMAGE ?= mgoltzsche/khelm:latest

BUILD_DIR := $(CURDIR)/build
BIN_DIR := $(BUILD_DIR)/bin
KHELM := $(BIN_DIR)/khelm
KHELM_STATIC := $(BIN_DIR)/khelm-static
GOLANGCI_LINT = $(BIN_DIR)/golangci-lint
KPT := $(BIN_DIR)/kpt
KUSTOMIZE := $(BIN_DIR)/kustomize

GOLANGCI_LINT_VERSION ?= v1.42.1
KPT_VERSION ?= v0.39.2
KUSTOMIZE_VERSION ?= v4.1.3
BATS_VERSION = v1.3.0

BATS_DIR = $(BUILD_DIR)/tools/bats
BATS = $(BIN_DIR)/bats

REV := $(shell git rev-parse --short HEAD 2> /dev/null || echo 'unknown')
VERSION ?= $(shell echo "$$(git describe --exact-match --tags $(git log -n1 --pretty='%h') 2> /dev/null || echo dev)-$(REV)" | sed 's/^v//')
HELM_VERSION := $(shell grep k8s\.io/helm go.mod | sed -E -e 's/k8s\.io\/helm|\s+|\+.*//g' -e 's/^v//' | cut -d " " -f2)
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

e2e-test: image khelm-static kpt kustomize | $(BATS)
	@echo 'Running e2e tests (PATH=$(BUILD_DIR)/bin)'
	@{ \
	export PATH="$(BIN_DIR):$$PATH" IMAGE=$(IMAGE); \
	$(BATS) -T ./e2e; \
	}

fmt:
	go fmt ./...

clean:
	rm -f $(BUILD_DIR)/bin/khelm
	rm -f $(BUILD_DIR)/bin/khelm-static
	rm -rf example/localrefref/charts
	rm -rf example/kpt/linkerd/dep

clean-all: clean
	rm -rf $(BUILD_DIR)
	find . -name charts -type d -exec rm -rf {} \;

check: $(GOLANGCI_LINT) ## Runs linters
	$(GOLANGCI_LINT) run ./...

kpt: $(KPT)

kustomize: $(KUSTOMIZE)

golangci-lint: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))

$(KPT):
	$(call download-bin,$(KPT),"https://github.com/GoogleContainerTools/kpt/releases/download/$(KPT_VERSION)/kpt_$$(uname | tr '[:upper:]' '[:lower:]')_amd64")

$(KUSTOMIZE):
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION))

$(BATS):
	@echo Downloading bats
	@{ \
	set -e ;\
	mkdir -p $(BIN_DIR) ;\
	TMP_DIR=$$(mktemp -d) ;\
	cd $$TMP_DIR ;\
	git clone -c 'advice.detachedHead=false' --branch $(BATS_VERSION) https://github.com/bats-core/bats-core.git . >/dev/null;\
	./install.sh $(BATS_DIR) ;\
	ln -s $(BATS_DIR)/bin/bats $(BATS) ;\
	}

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)/bin

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/build/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# download-bin downloads a binary into the location given as first argument
define download-bin
@[ -f $(1) ] || { \
set -e ;\
mkdir -p `dirname $(1)` ;\
TMP_FILE=$$(mktemp) ;\
echo "Downloading $(2)" ;\
curl -fsSLo $$TMP_FILE $(2) ;\
chmod +x $$TMP_FILE ;\
mv $$TMP_FILE $(1) ;\
}
endef
