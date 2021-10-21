module github.com/mgoltzsche/khelm/v2

go 1.16

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/ghodss/yaml v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.2.1
	github.com/stretchr/testify v1.7.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // must include https://github.com/go-yaml/yaml/issues/578
	helm.sh/helm/v3 v3.7.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/kustomize/kyaml v0.12.0
)
