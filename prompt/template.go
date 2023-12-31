package prompt

import (
	"bytes"
	"text/template"
)

const (
	PROMPT = `can u write unit test on golang with heights coverage and multi scenario for this code

{{.SourceCode}}

{{with .Structs}}
and i have some struct like this

{{range .}}
type {{.Name}} struct {
	{{range .Fields}}
	{{.Name}} {{.Type}}
	{{end}}
}
from {{.From}}
{{end}}

{{end}}

{{with .Mock}}
and i use mock {{.Name}} and the dir is {{.Dir}}
{{end}}

i expect the unit test like this
func Test_[function_name](t *testing.T) {

	// add some preparation code here

	// add schenario here with []struct

	// looping schenario here and test the function
}
	`
)

type Template struct {
	// Function string
	SourceCode string
	Structs    []Struct
	Mock       Mock
}

type Struct struct {
	Name   string
	From   string
	Fields []StructField
}

type StructField struct {
	Name string
	Type string
}

type Mock struct {
	Name string
	Dir  string
}

func (p Template) Generate() (string, error) {
	tmpl, err := template.New("prompt").Parse(PROMPT)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, p)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
