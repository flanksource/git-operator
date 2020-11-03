package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

type Dynamic struct {
	log           logr.Logger
	restMapper    meta.RESTMapper
	restConfig    *rest.Config
	dynamicClient dynamic.Interface
}

func NewDynamic(log logr.Logger) *Dynamic {
	return &Dynamic{log: log}
}

func (c *Dynamic) Apply(ctx context.Context, namespace string, obj runtime.Object) error {
	client, resource, unstructuredObj, err := c.GetDynamicClientFor(namespace, obj)
	if err != nil {
		c.log.Error(err, "failed to get dynamic client")
		return err
	}

	existing, _ := client.Get(ctx, unstructuredObj.GetName(), metav1.GetOptions{})
	if existing == nil {
		_, err = client.Create(ctx, unstructuredObj, metav1.CreateOptions{})
		if err != nil {
			c.log.Error(err, "error creating", "kind", unstructuredObj.GetKind(), "name", unstructuredObj.GetName())
			return err
		} else {
			c.log.Info("created", "kind", resource.Resource, "name", unstructuredObj.GetName())
		}
	} else {
		if unstructuredObj.GetKind() == "Service" {
			// Workaround for immutable spec.clusterIP error message
			spec := unstructuredObj.Object["spec"].(map[string]interface{})
			spec["clusterIP"] = existing.Object["spec"].(map[string]interface{})["clusterIP"]
		} else if unstructuredObj.GetKind() == "ServiceAccount" {
			unstructuredObj.Object["secrets"] = existing.Object["secrets"]
		}
		// apps/DameonSet MatchExpressions:[]v1.LabelSelectorRequirement(nil)}: field is immutable
		// webhook CA's

		unstructuredObj.SetResourceVersion(existing.GetResourceVersion())
		unstructuredObj.SetSelfLink(existing.GetSelfLink())
		unstructuredObj.SetUID(existing.GetUID())
		unstructuredObj.SetCreationTimestamp(existing.GetCreationTimestamp())
		unstructuredObj.SetGeneration(existing.GetGeneration())
		if existing.GetAnnotations() != nil && existing.GetAnnotations()["deployment.kubernetes.io/revision"] != "" {
			annotations := unstructuredObj.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["deployment.kubernetes.io/revision"] = existing.GetAnnotations()["deployment.kubernetes.io/revision"]
			unstructuredObj.SetAnnotations(annotations)
		}
		_, err := client.Update(ctx, unstructuredObj, metav1.UpdateOptions{})
		if err != nil {
			c.log.Error(err, "error updating", "kind", resource.Resource, "name", unstructuredObj.GetName())
			return err
		}

		c.log.Info("updated", "kind", resource.Resource, "name", unstructuredObj.GetName())
	}

	return nil
}

func (c *Dynamic) Delete(ctx context.Context, namespace string, obj runtime.Object) error {
	client, _, unstructuredObj, err := c.GetDynamicClientFor(namespace, obj)
	if err != nil {
		c.log.Error(err, "failed to get dynamic client")
		return err
	}

	if err := client.Delete(ctx, unstructuredObj.GetName(), metav1.DeleteOptions{}); err != nil {
		c.log.Error(err, "error deleting %s %s", "kind", unstructuredObj.GetKind(), "name", unstructuredObj.GetName())
		return err
	}

	c.log.Info("deleted %s %s", "kind", unstructuredObj.GetKind(), "name", unstructuredObj.GetName())
	return nil
}

func (c *Dynamic) GetDynamicClientFor(namespace string, obj runtime.Object) (dynamic.ResourceInterface, *schema.GroupVersionResource, *unstructured.Unstructured, error) {
	dynamicClient, err := c.GetDynamicClient()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("getDynamicClientFor: failed to get dynamic client: %v", err)
	}

	return c.getDynamicClientFor(dynamicClient, namespace, obj)
}

// GetDynamicClient creates a new k8s client
func (c *Dynamic) GetDynamicClient() (dynamic.Interface, error) {
	if c.dynamicClient != nil {
		return c.dynamicClient, nil
	}
	cfg, err := c.GetRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("getClientset: failed to get REST config: %v", err)
	}
	c.dynamicClient, err = dynamic.NewForConfig(cfg)
	return c.dynamicClient, err
}

func (c *Dynamic) getDynamicClientFor(dynamicClient dynamic.Interface, namespace string, obj runtime.Object) (dynamic.ResourceInterface, *schema.GroupVersionResource, *unstructured.Unstructured, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
	rm, _ := c.GetRestMapper()

	mapping, err := rm.RESTMapping(gk, gvk.Version)
	if err != nil && meta.IsNoMatchError(err) {
		// new CRD may still becoming ready, flush caches and retry
		time.Sleep(5 * time.Second)
		c.restMapper = nil
		rm, _ := c.GetRestMapper()
		mapping, err = rm.RESTMapping(gk, gvk.Version)
	}
	if err != nil {
		return nil, nil, nil, err
	}

	resource := mapping.Resource

	convertedObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("getDynamicClientFor: failed to convert object: %v", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: convertedObj}

	if mapping.Scope == meta.RESTScopeRoot {
		return dynamicClient.Resource(mapping.Resource), &resource, unstructuredObj, nil
	}
	if namespace == "" {
		namespace = unstructuredObj.GetNamespace()
	}
	return dynamicClient.Resource(mapping.Resource).Namespace(namespace), &resource, unstructuredObj, nil
}

func (c *Dynamic) GetRestMapper() (meta.RESTMapper, error) {
	if c.restMapper != nil {
		return c.restMapper, nil
	}

	config, err := c.GetRESTConfig()
	if err != nil {
		return nil, err
	}

	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	c.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
	return c.restMapper, nil
}

func (c *Dynamic) GetRESTConfig() (*rest.Config, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubeConfig, nil
}
