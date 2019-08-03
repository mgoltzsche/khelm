package helm

// See also https://github.com/kubernetes-sigs/kustomize/issues/880

import (
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/manifest"
)

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

func setNamespaceIfMissing(m *manifest.Manifest, defaultNamespace string) (err error) {
	if defaultNamespace == "" {
		return
	}
	obj := map[string]interface{}{}
	if err = yaml.Unmarshal([]byte(m.Content), &obj); err != nil {
		return errors.Wrap(err, "set missing namespace")
	}
	kind, hasKind := obj["kind"].(string)
	if !hasKind {
		return errors.Errorf("object has no kind of type string: %#v", obj)
	}
	meta, hasMeta := obj["metadata"].(map[string]interface{})
	if !hasMeta {
		return errors.Errorf("object has no metadata of type map[string]interface{}: %#v", obj)
	}
	if !nonNamespacedKinds[kind] && (meta["namespace"] == nil || meta["namespace"] == "") {
		meta["namespace"] = defaultNamespace
		obj["metadata"] = meta
		var b []byte
		if b, err = yaml.Marshal(obj); err != nil {
			return errors.Wrap(err, "set missing namespace")
		}
		m.Content = string(b)
	}
	return
}
