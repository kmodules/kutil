package v1

import (
	"sync"

	"github.com/appscode/kutil"
	"github.com/appscode/kutil/admission/api"
	"github.com/appscode/kutil/meta"
	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/api/batch/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type JobWebhook struct {
	client   kubernetes.Interface
	handler  api.ResourceHandler
	plural   schema.GroupVersionResource
	singular string

	initialized bool
	lock        sync.RWMutex
}

var _ api.AdmissionHook = &JobWebhook{}

func NewJobWebhook(plural schema.GroupVersionResource, singular string, handler api.ResourceHandler) *JobWebhook {
	return &JobWebhook{
		plural:   plural,
		singular: singular,
		handler:  handler,
	}
}

func (a *JobWebhook) Resource() (schema.GroupVersionResource, string) {
	return a.plural, a.singular
}

func (a *JobWebhook) Initialize(config *rest.Config, stopCh <-chan struct{}) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.initialized = true

	var err error
	a.client, err = kubernetes.NewForConfig(config)
	return err
}

func (a *JobWebhook) Admit(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	status := &admission.AdmissionResponse{}

	if a.handler == nil ||
		(req.Operation != admission.Create && req.Operation != admission.Update && req.Operation != admission.Delete) ||
		len(req.SubResource) != 0 ||
		(req.Kind.Group != v1.GroupName) ||
		req.Kind.Kind != "Job" {
		status.Allowed = true
		return status
	}

	a.lock.RLock()
	defer a.lock.RUnlock()
	if !a.initialized {
		return api.StatusUninitialized()
	}
	gv := schema.GroupVersion{Group: req.Kind.Group, Version: req.Kind.Version}

	switch req.Operation {
	case admission.Delete:
		// req.Object.Raw = nil, so read from kubernetes
		obj, err := a.client.BatchV1().Jobs(req.Namespace).Get(req.Name, metav1.GetOptions{})
		if err != nil && !kerr.IsNotFound(err) {
			return api.StatusInternalServerError(err)
		} else if err == nil {
			err2 := a.handler.OnDelete(obj)
			if err2 != nil {
				return api.StatusBadRequest(err)
			}
		}
	case admission.Create:
		v1Obj, originalObj, err := convert_to_v1_job(gv, req.Object.Raw)
		if err != nil {
			return api.StatusBadRequest(err)
		}

		v1Mod, err := a.handler.OnCreate(v1Obj)
		if err != nil {
			return api.StatusForbidden(err)
		} else if v1Mod != nil {
			patch, err := create_job_patch(gv, originalObj, v1Mod)
			if err != nil {
				return api.StatusInternalServerError(err)
			}
			status.Patch = patch
			patchType := admission.PatchTypeJSONPatch
			status.PatchType = &patchType
		}
	case admission.Update:
		v1Obj, originalObj, err := convert_to_v1_job(gv, req.Object.Raw)
		if err != nil {
			return api.StatusBadRequest(err)
		}
		v1OldObj, _, err := convert_to_v1_job(gv, req.OldObject.Raw)
		if err != nil {
			return api.StatusBadRequest(err)
		}

		v1Mod, err := a.handler.OnUpdate(v1OldObj, v1Obj)
		if err != nil {
			return api.StatusForbidden(err)
		} else if v1Mod != nil {
			patch, err := create_job_patch(gv, originalObj, v1Mod)
			if err != nil {
				return api.StatusInternalServerError(err)
			}
			status.Patch = patch
			patchType := admission.PatchTypeJSONPatch
			status.PatchType = &patchType
		}
	}

	status.Allowed = true
	return status
}

func convert_to_v1_job(gv schema.GroupVersion, raw []byte) (*v1.Job, runtime.Object, error) {
	switch gv {
	case v1.SchemeGroupVersion:
		v1Obj, err := meta.UnmarshalToJSON(raw, v1.SchemeGroupVersion)
		if err != nil {
			return nil, nil, err
		}
		return v1Obj.(*v1.Job), v1Obj, nil
	}
	return nil, nil, kutil.ErrUnknown
}

func create_job_patch(gv schema.GroupVersion, originalObj, v1Mod interface{}) ([]byte, error) {
	switch gv {
	case v1.SchemeGroupVersion:
		return meta.CreateJSONPatch(originalObj.(runtime.Object), v1Mod.(runtime.Object))
	}
	return nil, kutil.ErrUnknown
}
