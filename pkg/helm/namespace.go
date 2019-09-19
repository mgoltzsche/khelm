package helm

// See also https://github.com/kubernetes-sigs/kustomize/issues/880

import (
	"bytes"
	"io"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
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
	obj, err := parseObjects(bytes.NewReader([]byte(m.Content)))
	if err != nil {
		return errors.Errorf("%s: set namespace: %s: %q", filepath.Base(m.Name), err, m.Content)
	}
	modified := ""
	for _, o := range obj {
		kind, hasKind := o["kind"].(string)
		if !hasKind {
			return errors.Errorf("%s: object has no kind of type string: %#v", filepath.Base(m.Name), o)
		}
		meta, err := asMap(o["metadata"])
		if err != nil {
			return errors.Errorf("%s: object has no valid metadata: %#v", filepath.Base(m.Name), o)
		}
		if !nonNamespacedKinds[kind] && (meta["namespace"] == nil || meta["namespace"] == "") {
			meta["namespace"] = defaultNamespace
			o["metadata"] = meta
		}
		var b []byte
		if b, err = yaml.Marshal(o); err != nil {
			return errors.Wrap(err, "set missing namespace")
		}
		modified += "---\n" + string(b)
	}
	m.Content = modified
	return
}

func parseObjects(f io.Reader) (obj []map[string]interface{}, err error) {
	dec := yaml.NewDecoder(f)
	o := map[string]interface{}{}
	for ; err == nil; err = dec.Decode(o) {
		if len(o) > 0 {
			obj = append(obj, o)
			o = map[string]interface{}{}
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func asMap(o interface{}) (m map[string]interface{}, err error) {
	if o != nil {
		if mc, ok := o.(map[string]interface{}); ok {
			return mc, nil
		} else if mc, ok := o.(map[interface{}]interface{}); ok {
			m = map[string]interface{}{}
			for k, v := range mc {
				m[k.(string)] = v
			}
			return
		}
	}
	return nil, errors.New("invalid metadata")
}
