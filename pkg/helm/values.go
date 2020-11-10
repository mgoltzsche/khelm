package helm

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/proto/hapi/chart"
)

// vals merges values from files specified via -f/--values and
// directly via --set or --set-string or --set-file, marshaling them to YAML
func vals(chrt *chart.Chart, valueFiles []string, values map[string]interface{}, baseDir string, getters getter.Providers, certFile, keyFile, caFile string) (b []byte, err error) {
	base := map[string]interface{}{}
	for _, filePath := range valueFiles {
		currentMap := map[string]interface{}{}
		if b, err = readValuesFile(chrt, filePath, baseDir, getters, certFile, keyFile, caFile); err != nil {
			return
		}
		if err = yaml.Unmarshal(b, &currentMap); err != nil {
			return nil, errors.Wrapf(err, "failed to parse %s", filePath)
		}
		mergeValues(base, currentMap)
	}
	base = mergeValues(base, values)
	return yaml.Marshal(base)
}

// readValuesFile load a file from the local directory or a remote file with a url.
func readValuesFile(chrt *chart.Chart, filePath, baseDir string, getters getter.Providers, CertFile, KeyFile, CAFile string) (b []byte, err error) {
	u, err := url.Parse(filePath)
	if u.Scheme == "" || strings.ToLower(u.Scheme) == "file" {
		// Load from local file, fallback to chart file
		kustomizeFilePath := absPath(filePath, baseDir)
		if b, err = ioutil.ReadFile(kustomizeFilePath); os.IsNotExist(err) {
			// Fallback to chart file
			filePath = filepath.Clean(filePath)
			for _, f := range chrt.Files {
				if f.GetTypeUrl() == filePath {
					return f.GetValue(), nil
				}
			}
		}
		return
	} else if err != nil {
		return
	}

	// Load file from supported helm getter URL
	getterConstructor, err := getters.ByScheme(u.Scheme)
	if err != nil {
		return
	}
	getter, err := getterConstructor(filePath, CertFile, KeyFile, CAFile)
	if err != nil {
		return
	}
	data, err := getter.Get(filePath)
	return data.Bytes(), err
}

func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}
		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}
		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[string]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If we got to this point, it is a map in both, so merge them
		dest[k] = mergeValues(destMap, nextMap)
	}
	return dest
}
