helm-kustomize-plugin
[![Build Status](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin.svg?branch=master)](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin)
[![Go Report Card](https://goreportcard.com/badge/github.com/mgoltzsche/helm-kustomize-plugin)](https://goreportcard.com/report/github.com/mgoltzsche/helm-kustomize-plugin)
=

An experimental [kustomize](https://github.com/kubernetes-sigs/kustomize/)
plugin that allows to render [helm](https://github.com/helm/helm) charts into a kustomization.  

This plugin is an improved, golang-based version of
https://github.com/kubernetes-sigs/kustomize/tree/v3.1.0/plugin/someteam.example.com/v1/chartinflator
with helm built-in.

_THIS PROJECT IS STILL IN EARLY DEVELOPMENT._

## Motivation

[Helm](https://github.com/helm/helm) packages ("charts") provide a great way to
share and reuse kubernetes applications and there is a lot of them.
However there are some issues: For instance you cannot reuse a chart as is if
it does not (yet) support a particular parameter/value you need and
sometimes an additional `kubectl` command must be run (imperative!) to install
a chart that could [otherwise](https://docs.cert-manager.io/en/release-0.9/getting-started/install/kubernetes.html)
be installed as a single manifest using `kubectl`.  

[Kustomize](https://github.com/kubernetes-sigs/kustomize/) solves these issues by
simply merging [Kubernetes](https://github.com/kubernetes/kubernetes) API objects
which grants users of a _kustomization_ the freedom to change anything
and by supporting all API object kinds.  

With helm support kustomize can be used as a generic tool to render kubernetes manifests.


## Install

Install using curl (linux amd64):
```
mkdir -p $HOME/.config/kustomize/plugin/helm.mgoltzsche.github.com/v1/chartinflator
curl -L https://github.com/mgoltzsche/helm-kustomize-plugin/releases/latest/download/helm-kustomize-plugin > $HOME/.config/kustomize/plugin/helm.mgoltzsche.github.com/v1/chartinflator/ChartInflator
chmod u+x $HOME/.config/kustomize/plugin/helm.mgoltzsche.github.com/v1/chartinflator/ChartInflator
```
or using `go`:
```
go get github.com/mgoltzsche/helm-kustomize-plugin
mkdir -p $HOME/.config/kustomize/plugin/helm.mgoltzsche.github.com/v1/chartinflator
mv $GOPATH/bin/helm-kustomize-plugin $HOME/.config/kustomize/plugin/helm.mgoltzsche.github.com/v1/chartinflator/ChartInflator
```

The [kustomize plugin documentation](https://github.com/kubernetes-sigs/kustomize/tree/master/docs/plugins)
provides more information.


## Usage

A _plugin descriptor_ specifying the helm repository, chart, version and values
that should be used in a kubernetes-style resource can be referenced in the
`generators` section of a `kustomization.yaml` and looks as follows:
```
apiVersion: helm.mgoltzsche.github.com/v1
kind: ChartInflator
metadata:
  name: <NAME>
repository: <REPOSITORY>
chart: <CHART_NAME>
version: <CHART_VERSION>
valueFiles:
  - <VALUE_FILE>
value: <VALUE_MAP>
# TODO: ...
```

### Example

An example kustomization using this plugin can be found in the `example/jenkins` directory
and rendered like this:
```
kustomize build --enable_alpha_plugins github.com/mgoltzsche/helm-kustomize-plugin/example/jenkins
```


## Compatibility & security notice

Plugin support in kustomize is still an alpha feature and likely to be changed soon.  
Also this plugin's file system access is currently not restricted which allows
to load a helm value file from anywhere on your host. This may change as well.