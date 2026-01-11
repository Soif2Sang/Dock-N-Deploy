package utils

import (
	"regexp"
	"strings"
)

// SanitizeProjectName sanitizes the project name for Docker Compose
func SanitizeProjectName(name string) string {
	name = strings.ToLower(name)
	reg := regexp.MustCompile("[^a-z0-9]+")
	name = reg.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}
