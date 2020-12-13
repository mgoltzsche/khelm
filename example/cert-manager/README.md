This example kustomization renders the [cert-manager](https://github.com/jetstack/cert-manager) with CRDs and `Namespace´.  

It can be rendered and deployed as follows:
```
kustomize build --enable_alpha_plugins github.com/mgoltzsche/khelm/example/cert-manager | kubectl apply -f -
```
