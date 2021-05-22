# Example with Helm Chart Hooks

This is an example kustomization that renders a local Helm chart that contains [hooks](https://helm.sh/docs/topics/charts_hooks/).  

Corresponding to the `helm template` behaviour khelm returns all hook resources unless hooks are disabled explicitly.
