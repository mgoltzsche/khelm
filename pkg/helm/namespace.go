package helm

// See also https://github.com/kubernetes-sigs/kustomize/issues/880

var nonNamespacedKinds = func() map[string]bool {
	m := map[string]bool{}
	blacklist := []string{
		"ComponentStatus",
		"Namespace",
		"Node",
		"PersistentVolume",
		"MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration",
		"CustomResourceDefinition",
		"APIService",
		"MeshPolicy",
		"TokenReview",
		"SelfSubjectAccessReview",
		"SelfSubjectRulesReview",
		"SubjectAccessReview",
		"CertificateSigningRequest",
		"ClusterIssuer",
		"BGPConfiguration",
		"ClusterInformation",
		"FelixConfiguration",
		"GlobalBGPConfig",
		"GlobalFelixConfig",
		"GlobalNetworkPolicy",
		"GlobalNetworkSet",
		"HostEndpoint",
		"IPPool",
		"PodSecurityPolicy",
		"NodeMetrics",
		"PodSecurityPolicy",
		"ClusterRoleBinding",
		"ClusterRole",
		"ClusterRbacConfig",
		"PriorityClass",
		"StorageClass",
		"VolumeAttachment",
	}
	for _, kind := range blacklist {
		m[kind] = true
	}
	return m
}()

// ApplyDefaultNamespace sets the default namespace on all but the black-listed kinds
func (obj K8sObjects) ApplyDefaultNamespace(defaultNamespace string) {
	for _, o := range obj {
		meta, _ := asMap(o.Raw["metadata"])
		if !nonNamespacedKinds[o.Kind] && o.Namespace == "" {
			meta["namespace"] = defaultNamespace
			o.Raw["metadata"] = meta
			o.Namespace = defaultNamespace
		}
	}
}
