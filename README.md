# khelm ![GitHub workflow badge](https://github.com/mgoltzsche/khelm/workflows/Release/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/mgoltzsche/khelm)](https://goreportcard.com/report/github.com/mgoltzsche/khelm)

A [Helm](https://github.com/helm/helm) chart templating CLI, helm to kustomize converter, [kpt](https://github.com/GoogleContainerTools/kpt) function and [kustomize](https://github.com/kubernetes-sigs/kustomize/) plugin.  

Formerly known as "helm-kustomize-plugin".


## Motivation / History

[Helm](https://github.com/helm/helm) _charts_ provide a great way to share and reuse [Kubernetes](https://github.com/kubernetes/kubernetes) applications and there is a lot of them.
However writing helm templates is cumbersome and you cannot reuse a chart properly if it does not yet support a particular parameter/value.

[Kustomize](https://github.com/kubernetes-sigs/kustomize/) solves these issues declaratively by merging Kubernetes API objects which grants users of a _kustomization_ the freedom to change anything.
However kustomize neither supports lifecycle management nor templating with externally passed in values (which is sometimes still required).  

To overcome the gap between helm and kustomize initially this repository provided a kustomize plugin and [k8spkg](https://github.com/mgoltzsche/k8spkg) was used for lifecycle management.  
Since [kpt](https://github.com/GoogleContainerTools/kpt) is [published](https://opensource.googleblog.com/2020/03/kpt-packaging-up-your-kubernetes.html) helm and kustomize can be run as (chained) kpt functions supporting declarative, GitOps-based workflows. kpt also supports dynamic modification of static (rendered) manifests with externally passed in values using [setters](https://googlecontainertools.github.io/kpt/guides/consumer/set/) as well as [dependency](https://googlecontainertools.github.io/kpt/reference/pkg/) and [lifecycle management](https://googlecontainertools.github.io/kpt/reference/live/).


## Features

* Templates/renders a Helm chart
* Builds local charts automatically when templating
* Automatically fetches and updates required repository index files when needed
* Allows to automatically reload dependencies when lock file is out of sync
* Allows to use any repository without registering it in repositories.yaml
* Allows to exclude certain resources from the Helm chart output
* Allows to enforce namespace-scoped resources within the template output
* Allows to enforce a namespace on all resources
* Allows to convert a chart's output into a kustomization

## Supported interfaces

khelm can be used as:
* [kpt function](#kpt-function) (recommended)
* [kustomize exec plugin](#kustomize-exec-plugin)
* [CLI](#cli)
* [Go API](#go-api)

Usage examples can be found in the [example](example) and [e2e](e2e) directories.

### kpt function

The khelm kpt function templates a chart and returns the output as single manifest file or kustomization directory (when `outputPath` ends with `/`). The kustomization output can be used to apply further transformations by running a kustomize function afterwards.  

In opposite to the kustomize plugin approach kpt function outputs can be audited reliably when committed to a git repository, a kpt function does not depend on particular plugin binaries on the host and CD pipelines can run without dependencies to rendering technologies and chart servers since they just apply static mainfests (and eventually change values using `kpt cfg set`) to a cluster using `kpt live apply`.

#### kpt function usage example

A kpt function can be declared as annotated _ConfigMap_ within a kpt project.
A kpt project can be initialized and used with such a function as follows:
```sh
mkdir example-project && cd example-project
kpt pkg init . # Creates the Kptfile
cat - > khelm-function.yaml <<-EOF
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: cert-manager-manifest-generator
    annotations:
      config.kubernetes.io/function: |
        container:
          image: mgoltzsche/khelm:latest
          network: true
      config.kubernetes.io/local-config: "true"
  data:
    repository: https://charts.jetstack.io
    chart: cert-manager
    version: 0.9.x
    name: my-cert-manager-release
    namespace: cert-manager
    values:
      webhook:
        enabled: false
    outputPath: output-manifest.yaml
EOF
kpt fn run --network . # Renders the chart into output-manifest.yaml
```
_For all available fields see the [table](#configuration-options) below._  

Please note that, in case you need to refer to a local chart directory or values file, the source must be mounted to the function using `kpt fn run --mount=<SRC_MOUNT> .`.  
An [example kpt project](example/kpt/test-cases) and the corresponding [e2e test](e2e/kpt-function-test.sh) show how to do that.  

Kpt can also be leveraged to pull charts from other git repositories into your own repository using the `kpt pkg sync .` [command](https://googlecontainertools.github.io/kpt/reference/pkg/) (with a corresponding dependency set up) before running the khelm function (for this reason the go-getter support has been removed from this project).  

If necessary the chart output can be transformed using kustomize.
This can be done by declaring the khelm and a kustomize function orderly within a file and specifying the chart output kustomization as input for the kustomize function as shown in the [cert-manager example](example/kpt/cert-manager).
A more complex example that also manages a Helm chart from another git repository locally as kpt dependency can be found [here](example/kpt/linkerd).

#### Caching Helm Charts and repository index files

When external Helm Charts are used the download of their repositories' index files and of the charts itself can take a significant amount of time that adds up when running multiple functions or calling a function frequently during development.  
To speed this up caching can be enabled by mounting a host directory to `/helm` within the function container as shown [here](example/kpt/cache-dependencies).

### kustomize exec plugin

khelm can be used as [kustomize](https://github.com/kubernetes-sigs/kustomize) 3 [exec plugin](https://kubectl.docs.kubernetes.io/guides/extending_kustomize/execpluginguidedexample/).
Though plugin support in kustomize is still an alpha feature and may be removed in a future version.

#### Plugin installation

Install using curl (linux amd64):
```sh
mkdir -p $HOME/.config/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer
curl -fsSL https://github.com/mgoltzsche/khelm/releases/latest/download/khelm-linux-amd64 > $HOME/.config/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer/ChartRenderer
chmod u+x $HOME/.config/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer/ChartRenderer
```
or using `go`:
```sh
go get github.com/mgoltzsche/khelm/v2/cmd/khelm
mkdir -p $HOME/.config/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer
mv $GOPATH/bin/khelm $HOME/.config/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer/ChartRenderer
```

#### Plugin usage example

A _plugin descriptor_ specifies the helm repository, chart, version and values that should be used in a kubernetes-style resource can be referenced in the `generators` section of a `kustomization.yaml` and can look as follows:
```yaml
apiVersion: khelm.mgoltzsche.github.com/v2
kind: ChartRenderer
metadata:
  name: cert-manager # fallback for `name`
  namespace: cert-manager # fallback for `namespace`
repository: https://charts.jetstack.io
chart: cert-manager
version: 0.9.x
values:
  webhook:
    enabled: false
```
_For all available fields see the [table](#configuration-options) below._

More complete examples can be found within the [example](example) directory.
For instance `cert-manager` can be rendered like this:
```sh
kustomize build --enable_alpha_plugins github.com/mgoltzsche/khelm/example/cert-manager
```

### CLI

khelm also supports a helm-like `template` CLI.

#### Binary installation
```sh
curl -fsSL https://github.com/mgoltzsche/khelm/releases/latest/download/khelm-linux-amd64 > khelm
chmod +x khelm
sudo mv khelm /usr/local/bin/khelm
```

#### Binary usage example
```sh
khelm template cert-manager --version=0.9.x --repo=https://charts.jetstack.io
```
_For all available options see the [table](#configuration-options) below._

#### Docker usage example
```sh
docker run mgoltzsche/khelm:latest template cert-manager --version=0.9.x --repo=https://charts.jetstack.io
```

### Go API

The khelm Go API `github.com/mgoltzsche/khelm/v2/pkg/helm` provides a simple templating interface on top of the Helm Go API.
It exposes a `Helm` struct that provides a `Render()` function that returns the rendered resources as `kyaml` objects.

## Configuration options

| Field | CLI        | Description |
| ----- | ---------- | ----------- |
| `chart` | ARGUMENT    | Chart file (if `repository` not set) or name. |
| `version` | `--version` | Chart version. Latest version is used if not specified. |
| `repository` | `--repo` | URL to the repository the chart should be loaded from. |
| `valueFiles` | `-f` | Locations of values files.
| `values` | `--set` | Set values object or in CLI `key1=val1,key2=val2`. |
| `apiVersions` | `--api-versions` | Kubernetes api versions used for Capabilities.APIVersions. |
| `kubeVersion` | `--kube-version` | Kubernetes version used for Capabilities.KubeVersion. |
| `name` | `--name` | Release name used to render the chart. |
| `verify` | `--verify` | If enabled verifies the signature of all charts using the `keyring` (see [Helm 3 provenance and integrity](https://helm.sh/docs/topics/provenance/)). |
| `keyring` | `--keyring` | GnuPG keyring file (default `~/.gnupg/pubring.gpg`). |
| `replaceLockFile` | `--replace-lock-file` | Remove requirements.lock and reload charts when it is out of sync. |
| `excludeCRDs` | `--skip-crds` | If true Custom Resource Definitions are excluded from the output. |
| `include` |  | List of resource selectors that include matching resources from the output. If no selector specified all resources are included. Fails if a selector doesn't match any resource. Inclusions precede exclusions. |
| `include[].apiVersion` |  | Includes resources by apiVersion. |
| `include[].kind` |  | Includes resources by kind. |
| `include[].namespace` |  | Includes resources by namespace. |
| `include[].name` |  | Includes resources by name. |
| `exclude` |  | List of resource selectors that exclude matching resources from the output. Fails if a selector doesn't match any resource. |
| `exclude[].apiVersion` |  | Excludes resources by apiVersion. |
| `exclude[].kind` |  | Excludes resources by kind. |
| `exclude[].namespace` |  | Excludes resources by namespace. |
| `exclude[].name` |  | Excludes resources by name. |
| `namespace` | `--namespace` | Set the namespace used by Helm templates. |
| `namespacedOnly` | `--namespaced-only` | If enabled fail on known cluster-scoped resources and those of unknown kinds. |
| `forceNamespace` | `--force-namespace` | Set namespace on all namespaced resources (and those of unknown kinds). |
| `outputPath` | `--output` | Path to write the output to. If it ends with `/` a kustomization is generated. (Not supported by the kustomize plugin.) |
| `outputPathMapping[].outputPath` |  | output path to which all resources should be written that match `resourceSelectors`. (Only supported by the kpt function.) |
| `outputPathMapping[].selectors[].apiVersion` |  | Selects resources by apiVersion. |
| `outputPathMapping[].selectors[].kind` |  | Selects resources by kind. |
| `outputPathMapping[].selectors[].namespace` |  | Selects resources by namespace. |
| `outputPathMapping[].selectors[].name` |  | Selects resources by name. |
|  | `--output-replace` | If enabled replace the output directory or file (CLI-only). |
|  | `--trust-any-repo` | If enabled repositories that are not registered within `repositories.yaml` can be used as well (env var `KHELM_TRUST_ANY_REPO`). Within the kpt function this behaviour can be disabled by mounting `/helm/repository/repositories.yaml` or disabling network access. |
| `debug` | `--debug` | Enables debug log and provides a stack trace on error. |

### Repository configuration

Repository credentials can be configured using Helm's `repositories.yaml` which can be passed through as `Secret` to generic build jobs. khelm downloads the corresponding repo index files when needed.  

When running khelm as kpt function or within a container the `repositories.yaml` should be mounted to `/helm/repository/repositories.yaml`.  

Unlike Helm khelm allows usage of any repository when `repositories.yaml` is not present or `--trust-any-repo` is enabled.

## Helm support

* Helm 2 is supported by the `v1` module version.
* Helm 3 is supported by the `v2` module version.

## Build and test

Build and test the khelm binary (requires Go 1.13) as well as the container image:
```sh
make clean khelm test check image e2e-test
```
_The dynamic binary is written to `build/bin/khelm` and the static binary to `build/bin/khelm-static`_.

Alternatively a static binary can be built using `docker`:
```sh
make khelm-static
```

Install the binary on your host at `/usr/local/bin/khelm`:
```sh
sudo make install
```
