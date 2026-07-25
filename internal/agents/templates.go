package agents

import "strings"

// ExpandTemplate replaces {variable} placeholders in template with values from
// params. Placeholders that have no corresponding key in params are left as-is.
func ExpandTemplate(template string, params map[string]string) string {
	for k, v := range params {
		template = strings.ReplaceAll(template, "{"+k+"}", v)
	}
	return template
}
