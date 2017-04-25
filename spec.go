package lxdimage

import "github.com/juju/errors"

// Spec defines an image specification.
type Spec struct {
	BaseImage string
	Alias     string
	Templates []Template
	Commands  []string
}

// Validate validates a Spec.
func (spec Spec) Validate() error {
	if spec.BaseImage == "" {
		return errors.New("BaseImage must be set")
	}
	if spec.Alias == "" {
		return errors.New("Alias must be set")
	}
	// TODO(axw) validate templates
	return nil
}

// UnmarshalYAML is part of the gopkg.in/yaml.v2.Unmarshaler interface.
func (spec *Spec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var specYAML specYAML
	if err := unmarshal(&specYAML); err != nil {
		return errors.Annotate(err, "unmarshalling spec from YAML")
	}
	out := Spec{
		BaseImage: specYAML.BaseImage,
		Alias:     specYAML.Alias,
		Commands:  specYAML.Commands,
		// Use cloud-init templates by default. If the YAML
		// defines templates (empty or not), we'll override
		// this.
		Templates: CloudInitTemplates,
	}
	if specYAML.Templates != nil {
		out.Templates = make([]Template, len(*specYAML.Templates))
		for i, tmpl := range *specYAML.Templates {
			out.Templates[i] = Template{
				Properties: tmpl.Properties,
				Template:   tmpl.Template,
				When:       tmpl.When,
				Path:       tmpl.Path,
				Content:    tmpl.Content,
			}
		}
	}
	if err := out.Validate(); err != nil {
		return err
	}
	*spec = out
	return nil
}

type specYAML struct {
	BaseImage string          `yaml:"base"`
	Alias     string          `yaml:"alias"`
	Templates *[]templateYAML `yaml:"templates,omitempty"`
	Commands  []string        `yaml:"commands"`
}

type templateYAML struct {
	Properties map[string]string `yaml:"properties,omitempty"`
	Template   string            `yaml:"template"`
	When       []string          `yaml:"when,omitempty"`
	Path       string            `yaml:"path"`
	Content    string            `yaml:"content"`
}
