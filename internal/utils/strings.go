package utils

import (
	"regexp"
	"strings"
)

func Slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")
	reg, _ := regexp.Compile("[^a-z0-9-]+")
	s = reg.ReplaceAllString(s, "")
	reg2, _ := regexp.Compile("-+")
	s = reg2.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func StripHTML(content string, limit int) string {
	re := regexp.MustCompile("<[^>]*>")
	plain := re.ReplaceAllString(content, " ")
	plain = strings.Join(strings.Fields(plain), " ")
	if len(plain) > limit {
		return plain[:limit] + "..."
	}
	return plain
}
