package helm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyDefaultNamespace(t *testing.T) {
	defaultNamespace := "defaultns"
	for _, c := range []struct {
		kind              string
		namespace         string
		expectedNamespace string
	}{
		{"ServiceAccount", "predefinedns", "predefinedns"},
		{"ServiceAccount", "", defaultNamespace},
		{"ClusterRoleBinding", "", ""},
	} {
		obj := K8sObjects([]*K8sObject{{
			K8sObjectID{
				APIVersion: "some/version",
				Kind:       c.kind,
				Name:       "aname",
				Namespace:  c.namespace,
			},
			map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "aname",
					"namespace": c.namespace,
				},
			},
		}})
		obj.ApplyDefaultNamespace(defaultNamespace)
		for _, o := range obj {
			require.Equal(t, c.kind, o.Kind, "kind")
			require.Equal(t, c.expectedNamespace, o.Namespace, "invalid namespace (kind: %s)", c.kind)
		}
	}
}
