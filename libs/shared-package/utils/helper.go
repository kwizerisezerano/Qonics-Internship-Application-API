package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"reflect"
	"strings"
)

// SecureRandString returns a cryptographically secure random hex string built
// from n random bytes (2n hex characters). It mirrors leazi's helper of the
// same name and is used for single-use tokens such as password resets.
func SecureRandString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

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

// HumanizeStatus turns a machine status value into a human-readable, title-cased
// label for display in notifications and emails, e.g. "under_review" ->
// "Under Review".
func HumanizeStatus(status string) string {
	words := strings.Fields(strings.ReplaceAll(strings.TrimSpace(status), "_", " "))
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
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
