package helm

import (
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/releaseutil"
)

func TestHelm(t *testing.T) {
	defaultNamespace := "defaultns"
	for _, c := range []struct {
		kind              string
		namespace         string
		expectedNamespace interface{}
	}{
		{"ServiceAccount", "predefinedns", "predefinedns"},
		{"ServiceAccount", "", defaultNamespace},
		{"ClusterRoleBinding", "", nil},
	} {
		content := "apiVersion: some/version\nkind: " + c.kind + "\nmetadata:\n  name: aname\n"
		if c.namespace != "" {
			content += "  namespace: " + c.namespace
		}
		m := manifest.Manifest{Content: content, Head: &releaseutil.SimpleHead{Kind: c.kind}}
		setNamespaceIfMissing(&m, defaultNamespace)
		o := map[string]interface{}{}
		err := yaml.Unmarshal([]byte(m.Content), &o)
		require.NoError(t, err)
		namespace := o["metadata"].(map[string]interface{})["namespace"]
		require.Equal(t, c.expectedNamespace, namespace, "kind: %s", c.kind)
	}
}
