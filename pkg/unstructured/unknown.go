package unstructured

import (
	"fmt"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "helm-chartsnap.jlandowner.dev", Version: "v1alpha1"}
)

func NewUnknownError(raw string) *UnknownError {
	return &UnknownError{Raw: raw}
}

type UnknownError struct {
	Raw string
}

func (e *UnknownError) Error() string {
	return fmt.Sprintf("WARN: failed to recognize a resource in stdout/stderr of helm template command output. snapshot it as Unknown: \n---\n%s\n---", e.Raw)
}

func (e *UnknownError) Unstructured() *metaV1.Unstructured {
	return &metaV1.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": GroupVersion.String(),
			"kind":       "Unknown",
			"raw":        e.Raw,
		},
	}
}
