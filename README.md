helm-kustomize-plugin
[![Build Status](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin.svg?branch=master)](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin)
[![Go Report Card](https://goreportcard.com/badge/github.com/mgoltzsche/helm-kustomize-plugin)](https://goreportcard.com/report/github.com/mgoltzsche/helm-kustomize-plugin)
=

An experimental [kustomize](https://github.com/kubernetes-sigs/kustomize/)
plugin that allows to render [helm](https://github.com/helm/helm) charts into a kustomization.  

This plugin is an improved, golang-based version of the
[example chartinflator plugin](https://github.com/kubernetes-sigs/kustomize/tree/v3.1.0/plugin/someteam.example.com/v1/chartinflator)
with helm built-in.

## Motivation

[Helm](https://github.com/helm/helm) packages ("charts") provide a great way to
share and reuse kubernetes applications and there is a lot of them.
However there are some issues: For instance you cannot reuse a chart as is if
it does not (yet) support a particular parameter/value you need and
sometimes an additional `kubectl` command must be run (imperative!) to install
a chart that could [otherwise](https://docs.cert-manager.io/en/release-0.9/getting-started/install/kubernetes.html)
be installed as a single manifest using `kubectl`.
The _tiller_ server that comes with helm has certain
[issues](https://medium.com/virtuslab/think-twice-before-using-helm-25fbb18bc822)
as well. Though [helm 3 will provide some solutions](https://sweetcode.io/a-first-look-at-the-helm-3-plan/) to these.

[Kustomize](https://github.com/kubernetes-sigs/kustomize/) solves these issues already
declaratively by simply merging [Kubernetes](https://github.com/kubernetes/kubernetes)
API objects (which grants users of a _kustomization_ the freedom to change anything),
by supporting all API object kinds and focussing on serverless rendering.  

With helm support kustomize can be used as a generic tool to render kubernetes manifests.
These can be applied using `kubectl` directly or using [k8spkg](https://github.com/mgoltzsche/k8spkg)
which allows to manage their state within the cluster as well.


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
value: <VALUE_MAP>
```

### Example

Example kustomizations using this plugin can be found in the `example` directory.
For instance `cert-manager` can be rendered and deployed like this:
```
kustomize build --enable_alpha_plugins github.com/mgoltzsche/helm-kustomize-plugin/example/cert-manager | kubectl apply -f -
```


## Compatibility & security notice

Plugin support in kustomize is still an alpha feature and likely to be changed soon.