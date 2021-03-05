module github.com/mgoltzsche/khelm/v2

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/ghodss/yaml v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.7.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // must include https://github.com/go-yaml/yaml/issues/578
	helm.sh/helm/v3 v3.5.2
	k8s.io/client-go v0.20.2
	sigs.k8s.io/kustomize/kyaml v0.10.13
)

// See https://github.com/helm/helm/blob/v3.5.2/go.mod
replace (
	github.com/docker/distribution => github.com/docker/distribution v0.0.0-20191216044856-a8371794149d
	github.com/docker/docker => github.com/moby/moby v17.12.0-ce-rc1.0.20200618181300-9dc6525e6118+incompatible
)
