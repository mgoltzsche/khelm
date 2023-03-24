IMAGE ?= mgoltzsche/khelm:latest

BUILD_DIR := $(CURDIR)/build
BIN_DIR := $(BUILD_DIR)/bin
KHELM := $(BIN_DIR)/khelm
GOLANGCI_LINT = $(BIN_DIR)/golangci-lint
GORELEASER = $(BIN_DIR)/goreleaser
KPT := $(BIN_DIR)/kpt
KUSTOMIZE := $(BIN_DIR)/kustomize
SOPS := $(BIN_DIR)/sops
export HELM_SECRETS_SOPS_BIN := $(SOPS)
export HELM_PLUGINS := $(BUILD_DIR)/helm-plugins

GORELEASER_VERSION ?= v1.9.2
GOLANGCI_LINT_VERSION ?= v1.51.2
# TODO: update kpt when panic is fixed: https://github.com/GoogleContainerTools/kpt/issues/3868
KPT_VERSION ?= v1.0.0-beta.20
KUSTOMIZE_VERSION ?= v4.5.5
BATS_VERSION = v1.7.0
SOPS_VERSION = v3.7.3
HELM_SECRETS_VERSION = v3.14.0

BATS_DIR = $(BUILD_DIR)/tools/bats
BATS = $(BIN_DIR)/bats

REV := $(shell git rev-parse --short HEAD 2> /dev/null || echo 'unknown')
VERSION ?= $(shell echo "$$(git describe --exact-match --tags $(git log -n1 --pretty='%h') 2> /dev/null || echo dev)-$(REV)" | sed 's/^v//')
HELM_VERSION := $(shell grep helm\.sh/helm/ go.mod | sed -E -e 's!helm\.sh/helm/v3|\s+|\+.*!!g; s!^v!!' | cut -d " " -f2 | grep -E .+)
GO_LDFLAGS := -X main.khelmVersion=$(VERSION) -X main.helmVersion=$(HELM_VERSION) -s -w -extldflags '-static'
BUILDTAGS ?= 
CGO_ENABLED ?= 0
DOCKER ?= docker

all: clean khelm test check

khelm:
	CGO_ENABLED=$(CGO_ENABLED) go build -o $(BUILD_DIR)/bin/khelm -ldflags "$(GO_LDFLAGS)" -tags "$(BUILDTAGS)" ./cmd/khelm

install:
	cp $(BUILD_DIR)/bin/khelm /usr/local/bin/khelm
	chmod +x /usr/local/bin/khelm

install-kustomize-plugin:
	mkdir -p $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer
	cp $(BUILD_DIR)/bin/khelm $${XDG_CONFIG_HOME:-$$HOME/.config}/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer/ChartRenderer

image: khelm
	$(DOCKER) build --force-rm -t $(IMAGE) -f ./Dockerfile $(BIN_DIR)

test: $(BUILD_DIR) sops helm-plugins
	go test -coverprofile $(BUILD_DIR)/coverage.out -cover ./...

coverage: test
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html

e2e-test: kpt kustomize | $(BATS)
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
	# TODO: fix "invalid trailing UTF-8 octet" yaml parser error
	rm -f example/kpt/test-cases/output-remote.yaml

clean-all: clean
	rm -rf $(BUILD_DIR)
	find . -name charts -type d -exec rm -rf {} \;

check: $(GOLANGCI_LINT) ## Runs linters
	$(GOLANGCI_LINT) run ./...

snapshot: $(GORELEASER) ## Builds a snapshot release but does not publish it
	HELM_VERSION="$(HELM_VERSION)" $(GORELEASER) release --snapshot --rm-dist

register-qemu-binfmt: ## Enable multiarch support on the host
	$(DOCKER) run --rm --privileged multiarch/qemu-user-static:7.0.0-7 --reset -p yes

kpt: $(KPT)

sops: $(SOPS)

helm-plugins: $(HELM_PLUGINS)

kustomize: $(KUSTOMIZE)

golangci-lint: $(GOLANGCI_LINT)

goreleaser: $(GORELEASER)

$(GOLANGCI_LINT):
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))

$(KPT):
	$(call download-bin,$(KPT),"https://github.com/GoogleContainerTools/kpt/releases/download/$(KPT_VERSION)/kpt_$$(uname | tr '[:upper:]' '[:lower:]')_amd64")

$(KUSTOMIZE):
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION))

$(GORELEASER):
	$(call go-get-tool,$(GORELEASER),github.com/goreleaser/goreleaser@$(GORELEASER_VERSION))

$(SOPS):
	$(call download-bin,$(SOPS),"https://github.com/mozilla/sops/releases/download/$(SOPS_VERSION)/sops-$(SOPS_VERSION).$$(uname | tr '[:upper:]' '[:lower:]').amd64")

$(HELM_PLUGINS):
	$(call download-tar-gz,$(HELM_PLUGINS),"https://github.com/jkroepke/helm-secrets/releases/download/$(HELM_SECRETS_VERSION)/helm-secrets.tar.gz")

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
GOBIN=$(PROJECT_DIR)/build/bin go install $(2) ;\
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

# download-tar-gz downloads a tar.gz or .tgz file and extracts it into the location given as first argument
define download-tar-gz
@[ -d $(1) ] || { \
set -e ;\
echo $(1) ;\
mkdir -p $(1) ;\
cd $(1) ;\
echo "Downloading $(2)" ;\
curl -fsSLo downloaded.tar.gz $(2) ;\
tar -xzf downloaded.tar.gz ;\
rm downloaded.tar.gz ;\
}
endef
