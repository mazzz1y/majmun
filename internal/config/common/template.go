package common

import (
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

type Template template.Template

func (t *Template) UnmarshalYAML(value *yaml.Node) error {
	var tmplStr string
	if err := value.Decode(&tmplStr); err != nil {
		return err
	}
	tmpl, err := template.New("yaml_template").Funcs(sprig.FuncMap()).Parse(tmplStr)
	if err != nil {
		return err
	}

	*t = Template(*tmpl)
	return nil
}

func (t *Template) ToTemplate() *template.Template {
	tmpl := template.Template(*t)
	return &tmpl
}
