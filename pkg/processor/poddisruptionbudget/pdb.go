package poddisruptionbudget

import (
	"bytes"
	"fmt"
	"io"

	"github.com/MakeNowJust/heredoc"
	"github.com/arttor/helmify/pkg/helmify"
	"github.com/arttor/helmify/pkg/processor"
	yamlformat "github.com/arttor/helmify/pkg/yaml"
	"github.com/iancoleman/strcase"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

const (
	pdbTempSpec = `
spec:
  {{ with .Values.%[1]s.minAvailable -}}
  minAvailable: {{ . }}
  {{ end -}}
  {{ with .Values.%[1]s.maxUnavailable -}}
  maxUnavailable: {{ . }}
  {{ end -}}
  selector:
%[2]s
    {{- include "%[3]s.selectorLabels" . | nindent 6 }}`
	pdbWrapperSpec = `
	  {{ if .Values.%[1]s.enabled }}
	  %[2]s
	  {{ end }}
	`
)

var pdbGVC = schema.GroupVersionKind{
	Group:   "policy",
	Version: "v1",
	Kind:    "PodDisruptionBudget",
}

// New creates processor for k8s Service resource.
func New() helmify.Processor {
	return &pdb{}
}

type pdb struct{}

// Process k8s Service object into template. Returns false if not capable of processing given resource type.
func (r pdb) Process(appMeta helmify.AppMetadata, obj *unstructured.Unstructured) (bool, helmify.Template, error) {
	if obj.GroupVersionKind() != pdbGVC {
		return false, nil, nil
	}
	pdb := policyv1.PodDisruptionBudget{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pdb)
	if err != nil {
		return true, nil, fmt.Errorf("%w: unable to cast to pdb", err)
	}
	spec := pdb.Spec
	values := helmify.Values{}

	meta, err := processor.ProcessObjMeta(appMeta, obj)
	if err != nil {
		return true, nil, err
	}

	name := appMeta.TrimName(obj.GetName())
	nameCamel := strcase.ToLowerCamel(name)

	selector, _ := yaml.Marshal(pdb.Spec.Selector)
	selector = yamlformat.Indent(selector, 4)
	selector = bytes.TrimRight(selector, "\n ")

	setOpt := func(k string, v *intstr.IntOrString) error {
		r := ""
		if v != nil {
			r = v.String()
		}
		_, err := values.Add(r, nameCamel, k)
		return err
	}

	if err := setOpt("maxUnavailable", spec.MaxUnavailable); err != nil {
		return true, nil, err
	}

	if err := setOpt("minAvailable", spec.MinAvailable); err != nil {
		return true, nil, err
	}

	res := meta + fmt.Sprintf(pdbTempSpec, nameCamel, selector, appMeta.ChartName())
	res = heredoc.Docf(pdbWrapperSpec, nameCamel, res)
	if _, err := values.Add(true, nameCamel, "enabled"); err != nil {
		return true, nil, err
	}

	return true, &result{
		name:   name,
		data:   res,
		values: values,
	}, nil
}

type result struct {
	name   string
	data   string
	values helmify.Values
}

func (r *result) Filename() string {
	return r.name + ".yaml"
}

func (r *result) Values() helmify.Values {
	return r.values
}

func (r *result) Write(writer io.Writer) error {
	_, err := writer.Write([]byte(r.data))
	return err
}
