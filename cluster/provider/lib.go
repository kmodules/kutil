/*
Copyright AppsCode Inc. and Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"context"
	"errors"

	v1alpha1 "kmodules.xyz/client-go/apis/management/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DetectCAPICluster(kc client.Client) (*v1alpha1.CAPIClusterInfo, error) {
	var list unstructured.UnstructuredList
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})
	err := kc.List(context.TODO(), &list)
	if meta.IsNoMatchError(err) || len(list.Items) == 0 {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else if len(list.Items) > 1 {
		return nil, errors.New("multiple CAPI cluster object found")
	}

	obj := list.Items[0].UnstructuredContent()
	capiProvider, clusterName, ns, err := getCAPIValues(obj)
	if err != nil {
		return nil, err
	}

	return &v1alpha1.CAPIClusterInfo{
		Provider:    getProviderName(capiProvider),
		Namespace:   ns,
		ClusterName: clusterName,
	}, nil
}

func getCAPIValues(values map[string]any) (string, string, string, error) {
	capiProvider, ok, err := unstructured.NestedString(values, "spec", "infrastructureRef", "kind")
	if err != nil {
		return "", "", "", err
	} else if !ok || capiProvider == "" {
		return "", "", "", nil
	}

	clusterName, ok, err := unstructured.NestedString(values, "metadata", "name")
	if err != nil {
		return "", "", "", err
	} else if !ok || clusterName == "" {
		return "", "", "", nil
	}

	ns, ok, err := unstructured.NestedString(values, "metadata", "namespace")
	if err != nil {
		return "", "", "", err
	} else if !ok || ns == "" {
		return "", "", "", nil
	}

	return capiProvider, clusterName, ns, nil
}

func getProviderName(kind string) string {
	switch kind {
	case "AWSManagedControlPlane":
		return "capa"
	case "AzureManagedCluster":
		return "capz"
	case "GCPManagedCluster":
		return "capg"
	}
	return ""
}
