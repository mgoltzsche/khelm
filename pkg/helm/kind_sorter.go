package helm

import (
	"sort"

	"k8s.io/helm/pkg/manifest"
)

// SortOrder is an ordering of Kinds.
type sortOrder []string

// InstallOrder is the order in which manifests should be installed (by Kind).
//
// Those occurring earlier in the list get installed before those occurring later in the list.
// Copied from https://github.com/helm/helm/blob/v2.14.3/pkg/tiller/kind_sorter.go (since whole package has more dependencies and cannot be loaded as Go 1.11 Module).
var installOrder sortOrder = []string{
	"Namespace",
	"ResourceQuota",
	"LimitRange",
	"PodSecurityPolicy",
	"PodDisruptionBudget",
	"Secret",
	"ConfigMap",
	"StorageClass",
	"PersistentVolume",
	"PersistentVolumeClaim",
	"ServiceAccount",
	"CustomResourceDefinition",
	"ClusterRole",
	"ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"Service",
	"DaemonSet",
	"Pod",
	"ReplicationController",
	"ReplicaSet",
	"Deployment",
	"StatefulSet",
	"Job",
	"CronJob",
	"Ingress",
	"APIService",
}

type kindSorter struct {
	ordering  map[string]int
	manifests []manifest.Manifest
}

func newKindSorter(m []manifest.Manifest, s sortOrder) *kindSorter {
	o := make(map[string]int, len(s))
	for v, k := range s {
		o[k] = v
	}

	return &kindSorter{
		manifests: m,
		ordering:  o,
	}
}

func (k *kindSorter) Len() int { return len(k.manifests) }

func (k *kindSorter) Swap(i, j int) { k.manifests[i], k.manifests[j] = k.manifests[j], k.manifests[i] }

func (k *kindSorter) Less(i, j int) bool {
	a := k.manifests[i]
	b := k.manifests[j]
	first, aok := k.ordering[a.Head.Kind]
	second, bok := k.ordering[b.Head.Kind]

	if !aok && !bok {
		// if both are unknown then sort alphabetically by kind and name
		if a.Head.Kind != b.Head.Kind {
			return a.Head.Kind < b.Head.Kind
		}
		return a.Name < b.Name
	}

	// unknown kind is last
	if !aok {
		return false
	}
	if !bok {
		return true
	}

	// if same kind sub sort alphanumeric
	if first == second {
		return a.Name < b.Name
	}
	// sort different kinds
	return first < second
}

// SortByKind sorts manifests in InstallOrder
func sortByKind(manifests []manifest.Manifest) []manifest.Manifest {
	ks := newKindSorter(manifests, installOrder)
	sort.Sort(ks)
	return ks.manifests
}
