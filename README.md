helm-kustomize-plugin
[![Build Status](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin.svg?branch=master)](https://travis-ci.org/mgoltzsche/helm-kustomize-plugin)
[![Go Report Card](https://goreportcard.com/badge/github.com/mgoltzsche/helm-kustomize-plugin)](https://goreportcard.com/report/github.com/mgoltzsche/helm-kustomize-plugin)
=

An experimental kustomize plugin that allows to render helm charts into a kustomization.  

Having helm support in [kustomize](https://github.com/kubernetes-sigs/kustomize/)
allows to declaratively refer to charts and modify their rendered manifests
using a single kustomize call.  
This simplifies many deployments since it allows to build generic,
kustomize-based deployment pipelines while still supporting helm chart utilization.  

This plugin is an improved, golang-based version of
https://github.com/kubernetes-sigs/kustomize/tree/v3.1.0/plugin/someteam.example.com/v1/chartinflator.  

The `example/jenkins` directory shows how this plugin can be used.  

General kustomize plugin documentation: https://github.com/kubernetes-sigs/kustomize/tree/master/docs/plugins