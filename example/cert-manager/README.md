This example kustomization renders the [cert-manager](https://github.com/jetstack/cert-manager) crds, namespace and
helm chart without the webhook (to run on [k3s](https://github.com/rancher/k3s)).  

It can be rendered and deployed as follows:
```
kustomize build --enable_alpha_plugins github.com/mgoltzsche/helm-kustomize-plugin/example/cert-manager | kubectl apply -f -
```