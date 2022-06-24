This example kustomization renders the
[cert-manager](https://github.com/jetstack/cert-manager) with CRDs and
`Namespace´.  

It is identical to the example located at
https://github.com/mgoltzsche/khelm/example/cert-manager, except that it uses
a Containerized KRM Function instead of the Exec plugin that needs installation.

The annotations used to controll khelm are the same as those described in the
[README](https://github.com/mgoltzsche/khelm) for use with the kpt function in
the ConfigMap.

It can be rendered and deployed as follows:
```
kustomize build \
    --enable-alpha-plugins --network \
    github.com/mgoltzsche/khelm/example/cert-manager | kubectl apply -f -
```
_This strategy is not supported for kustomize versions < v4.1.0_
