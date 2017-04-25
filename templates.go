package lxdimage

const (
	cloudInitMetaTemplate = `#cloud-config
instance-id: {{ container.name }}
local-hostname: {{ container.name }}
{{ config_get("user.meta-data", "") }}`

	cloudInitNetworkTemplate = `{% if config_get("user.network-config", "") == "" %}version: 1
config:
    - type: physical
      name: eth0
      subnets:
          - type: {% if config_get("user.network_mode", "") == "link-local" %}manual{% else %}dhcp{% endif %}
            control: auto{% else %}{{ config_get("user.network-config", "") }}{% endif %}`

	cloudInitUserTemplate = `{{ config_get("user.user-data", properties.default) }}`

	cloudInitVendorTemplate = `{{ config_get("user.vendor-data", properties.default) }}`
)

// Template describes a LXD image template.
type Template struct {
	Properties map[string]string `yaml:"properties,omitempty"`
	Template   string            `yaml:"template"`
	When       []string          `yaml:"when,omitempty"`

	// Path is the path of the file on disk that the template
	// creates.
	Path string `yaml:"-"`

	// Content is the contents of the template file to create
	// in the image metadata.
	Content string `yaml:"-"`
}

// CloudInitTemplates contains a set of templates that can be used
// to create cloud-init metadata files.
var CloudInitTemplates = []Template{{
	Template: "cloud-init-meta.tpl",
	When:     []string{"create", "copy"},
	Path:     "/var/lib/cloud/seed/nocloud-net/meta-data",
	Content:  cloudInitMetaTemplate,
}, {
	Template: "cloud-init-network.tpl",
	When:     []string{"create", "copy"},
	Path:     "/var/lib/cloud/seed/nocloud-net/network-config",
	Content:  cloudInitNetworkTemplate,
}, {
	Properties: map[string]string{
		"default": "#cloud-config\n{}",
	},
	Template: "cloud-init-user.tpl",
	When:     []string{"create", "copy"},
	Path:     "/var/lib/cloud/seed/nocloud-net/user-data",
	Content:  cloudInitUserTemplate,
}, {
	Properties: map[string]string{
		"default": "#cloud-config\n{}",
	},
	Template: "cloud-init-vendor.tpl",
	When:     []string{"create", "copy"},
	Path:     "/var/lib/cloud/seed/nocloud-net/vendor-data",
	Content:  cloudInitVendorTemplate,
}}
