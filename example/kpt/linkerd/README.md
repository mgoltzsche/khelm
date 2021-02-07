# example: static linkerd manifest

This example project includes the [linkerd2](https://github.com/linkerd/linkerd2) charts as kpt dependency and converts them to a static manifest which does not contain any `Secrets` but [cert-manager](https://cert-manager.io) `Certificates` instead. The transformations are specified within `manifests/helm-kustomize-pipeline.yaml`.  

However currently the static manifest is not functional since linkerd does not allow to specify the `-identity-trust-anchors-pem` option as environment variable that could be loaded from a Secret (see the related [linkerd issue](https://github.com/linkerd/linkerd2/issues/3843)).

## Workflow

1) Update dependencies: `make update`
2) Generate the static manifest: `make manifest`
3) Commit the changes (including generated manifests) if any
4) Within a CD pipeline deploy the manifest: `make deploy` or rather `kpt live apply manifests/static` (the latter requires cert-manager to be installed beforehand)
