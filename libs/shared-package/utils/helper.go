package utils

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"reflect"
	"strings"
)

// EmailTemplatesDir is the directory that holds the HTML email templates. It
// mirrors the leazi worker layout (/app/templates) and can be overridden, e.g.
// in tests.
var EmailTemplatesDir = "/app/templates"

// GenerateHtmlTemplate renders a named HTML email template with the provided
// data. Every template is parsed together with the shared header.html and
// footer.html partials (declared as {{define "header"}} / {{define "footer"}})
// so branding stays consistent across all emails, mirroring the leazi worker
// pattern.
func GenerateHtmlTemplate(filename string, emailData any) (string, error) {
	filename = strings.TrimSuffix(filename, ".html")

	mainPath := fmt.Sprintf("%s/%s.html", EmailTemplatesDir, filename)
	headerPath := fmt.Sprintf("%s/header.html", EmailTemplatesDir)
	footerPath := fmt.Sprintf("%s/footer.html", EmailTemplatesDir)

	tmpl, err := template.New("").ParseFiles(mainPath, headerPath, footerPath)
	if err != nil {
		return "", errors.New("error parsing template: " + err.Error())
	}

	mainTemplate := tmpl.Lookup(filename + ".html")
	if mainTemplate == nil {
		return "", errors.New("template not found: " + filename + ".html")
	}

	var body bytes.Buffer
	if err := mainTemplate.Execute(&body, emailData); err != nil {
		return "", errors.New("error rendering template: " + err.Error())
	}
	return body.String(), nil
}

// AvailableSourceTable lists the tables that may own a file through the
// polymorphic source_table / source_table_id pair (leazi-style allow-list).
var AvailableSourceTable = []string{
	"applications",
}

// In_array reports whether value is present in slice. It mirrors the leazi
// helper of the same name so file-source validation stays consistent.
func In_array(slice any, value any) bool {
	s := reflect.ValueOf(slice)

	if s.Kind() != reflect.Slice {
		panic("slice value: is not a slice")
	}
	for i := 0; i < s.Len(); i++ {
		if reflect.DeepEqual(s.Index(i).Interface(), value) {
			return true
		}
	}
	return false
}
