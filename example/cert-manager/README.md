This example kustomization renders the [cert-manager](https://github.com/jetstack/cert-manager) with CRDs and `Namespace´.  

It can be rendered and deployed as follows:
```
kustomize build --enable-alpha-plugins github.com/mgoltzsche/khelm/example/cert-manager | kubectl apply -f -
```
_When using kustomize 3 the option is called `--enable_alpha_plugins`._
