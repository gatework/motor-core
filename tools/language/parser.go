package language

import (
	"strings"

	textlanguage "golang.org/x/text/language"
)

// ParseAcceptLanguage returns normalized language tags in preference order.
// Zero-quality tags are excluded. Invalid headers return an empty result.
// A non-empty supportedLanguages list restricts the returned tags.
func ParseAcceptLanguage(header string, supportedLanguages []string) []string {
	tags, _, err := textlanguage.ParseAcceptLanguage(strings.ReplaceAll(header, "_", "-"))
	if err != nil {
		return []string{}
	}
	supported := make(map[string]struct{}, len(supportedLanguages))
	for _, name := range supportedLanguages {
		tag, err := textlanguage.Parse(strings.ReplaceAll(name, "_", "-"))
		if err == nil {
			supported[strings.ToLower(tag.String())] = struct{}{}
		}
	}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		name := strings.ToLower(tag.String())
		if len(supportedLanguages) != 0 {
			if _, ok := supported[name]; !ok {
				continue
			}
		}
		result = append(result, name)
	}
	return result
}
