module github.com/mgoltzsche/khelm/v2

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.0.0
	github.com/stretchr/testify v1.6.1
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // must include https://github.com/go-yaml/yaml/issues/578
	helm.sh/helm/v3 v3.4.1
	k8s.io/client-go v0.19.3
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/kustomize/kyaml v0.10.1
)
