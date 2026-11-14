package language

import (
	"reflect"
	"testing"
)

// A browser may send either form of a locale tag, and the parser normalises
// the underscore one so that it can match a supported tag. The normalisation
// asked strings.Replace for zero replacements, so it never happened and the
// underscore form matched nothing.
func TestParseAcceptLanguageNormalisesUnderscores(t *testing.T) {
	cases := []struct {
		name      string
		header    string
		supported []string
		want      []string
	}{
		{
			name:      "an underscore tag matches its hyphen form",
			header:    "zh_CN",
			supported: []string{"zh-cn"},
			want:      []string{"zh-cn"},
		},
		{
			name:      "the hyphen form still matches",
			header:    "zh-CN",
			supported: []string{"zh-cn"},
			want:      []string{"zh-cn"},
		},
		{
			name:      "quality still orders the result",
			header:    "en;q=0.8,zh_CN;q=0.9",
			supported: []string{"zh-cn", "en"},
			want:      []string{"zh-cn", "en"},
		},
		{
			name:      "an unsupported tag is dropped",
			header:    "de_DE",
			supported: []string{"zh-cn"},
			want:      []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseAcceptLanguage(c.header, c.supported)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ParseAcceptLanguage(%q, %v) = %v, want %v", c.header, c.supported, got, c.want)
			}
		})
	}
}

func TestParseAcceptLanguageQuality(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		want         []string
	}{
		{"zero quality is excluded", "en;q=0,zh;q=1", []string{"zh"}},
		{"implicit quality is one", "en;q=1,fr,de;q=0.9", []string{"en", "fr", "de"}},
		{"equal quality preserves order", "fr;q=0.7,de;q=0.7,en;q=0.7", []string{"fr", "de", "en"}},
		{"invalid quality is rejected", "en;q=bad", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAcceptLanguage(tc.header, nil)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}
func TestParseAcceptLanguageNormalizesSupportedTags(t *testing.T) {
	got := ParseAcceptLanguage("zh_cn,en", []string{"ZH_CN"})
	if !reflect.DeepEqual(got, []string{"zh-cn"}) {
		t.Fatalf("got=%v", got)
	}
}
