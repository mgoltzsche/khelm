helm-kustomize-plugin
[![Build Status](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin.svg?branch=master)](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin)
[![Go Report Card](https://goreportcard.com/badge/github.com/mgoltzsche/helm-kustomize-plugin)](https://goreportcard.com/report/github.com/mgoltzsche/helm-kustomize-plugin)
=

An experimental [kustomize](https://github.com/kubernetes-sigs/kustomize/) plugin that allows to render [helm](https://github.com/helm/helm) charts into a kustomization.  

This plugin is an improved, golang-based version of the [example chartinflator plugin](https://github.com/kubernetes-sigs/kustomize/tree/v3.1.0/plugin/someteam.example.com/v1/chartinflator) with helm built-in.

## Motivation / History

[Helm](https://github.com/helm/helm) packages ("charts") provide a great way to share and reuse kubernetes applications and there is a lot of them.
However writing helm templates is cumbersome and you cannot reuse a chart properly if it does not (yet) support a particular parameter/value.

[Kustomize](https://github.com/kubernetes-sigs/kustomize/) solves these issues declaratively by merging [Kubernetes](https://github.com/kubernetes/kubernetes) API objects which grants users of a _kustomization_ the freedom to change anything.
However kustomize neither supports lifecycle management nor templating with externally passed in values (which is sometimes still required).  

To overcome the gap between helm and kustomize initially this repository provided a kustomize plugin and [k8spkg](https://github.com/mgoltzsche/k8spkg) was used for lifecycle management.
Since [kpt](https://github.com/GoogleContainerTools/kpt) is [published](https://opensource.googleblog.com/2020/03/kpt-packaging-up-your-kubernetes.html) both technologies can be used as (chained) kpt functions. kpt also supports simple templating of static (rendered) manifests using [setters](https://googlecontainertools.github.io/kpt/guides/consumer/set/), dependency and [lifecycle management](https://googlecontainertools.github.io/kpt/reference/live/).


## Requirements

* [kustomize](https://github.com/kubernetes-sigs/kustomize) 3.0.0


## Install

Install using curl (linux amd64):
```
mkdir -p $HOME/.config/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer
curl -L https://github.com/mgoltzsche/helm-kustomize-plugin/releases/latest/download/helm-kustomize-plugin > $HOME/.config/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer
chmod u+x $HOME/.config/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer
```
or using `go`:
```
go get github.com/mgoltzsche/helm-kustomize-plugin
mkdir -p $HOME/.config/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer
mv $GOPATH/bin/helm-kustomize-plugin $HOME/.config/kustomize/plugin/helm.kustomize.mgoltzsche.github.com/v1/chartrenderer/ChartRenderer
```

The [kustomize plugin documentation](https://github.com/kubernetes-sigs/kustomize/tree/master/docs/plugins)
provides more information.


## Usage

A _plugin descriptor_ specifying the helm repository, chart, version and values
that should be used in a kubernetes-style resource can be referenced in the
`generators` section of a `kustomization.yaml` and looks as follows:
```
apiVersion: helm.kustomize.mgoltzsche.github.com/v1
kind: ChartRenderer
metadata:
  name: <NAME>
  namespace: <NAMESPACE>
repository: <REPOSITORY>
chart: <CHART_NAME>
version: <CHART_VERSION>
valueFiles:
- <VALUE_FILE>
value: <VALUE_OBJECT>
apiVersions:
- <API_VERSION>
exclude:
- apiVersion: <APIVERSION>
  kind: <KIND>
  namespace: <NAMESPACE>
  name: <NAME>
```

* `repository`: a helm repository URL.
* `chart` (mandatory): chart name (using `repository`) or, when `repository` is not specified, the path to a local chart (which will be built recursively).
* `version`: chart version if a remote chart is specified.
* `valueFiles`: a list of helm value file paths relative to the generator config file or to the chart.
* `value`: a values object.
* `apiVersions`: a list of apiVersions used for Capabilities.APIVersions.
* `exclude`: a list of selectors used to exclude matching objects from the rendered chart.

### Example

Example kustomizations using this plugin can be found in the `example` directory.
For instance `cert-manager` can be rendered and deployed like this:
```
kustomize build --enable_alpha_plugins github.com/mgoltzsche/helm-kustomize-plugin/example/cert-manager | kubectl apply -f -
```


## Compatibility & security notice

Plugin support in kustomize is still an alpha feature.  

Helm charts may access the local file system outside the kustomization directory.
