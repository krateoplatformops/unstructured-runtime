package objectref

import "fmt"

type ObjectRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

func (o *ObjectRef) String() string {
	return fmt.Sprintf("%s.%s as %s@%s", o.APIVersion, o.Kind, o.Name, o.Namespace)
}
