package fixturebinder

import "strings"

// extensionFor maps a fixture content_type to the file extension used when
// writing its payload under ScratchDir. Mirrors fixture/payload.go's
// structured-content detection.
func extensionFor(contentType string) string {
	switch contentType {
	case "application/json":
		return ".json"
	case "application/yaml", "text/yaml", "application/x-yaml":
		return ".yaml"
	case "application/xml", "text/xml":
		return ".xml"
	}
	if strings.HasSuffix(contentType, "+json") {
		return ".json"
	}
	if strings.HasSuffix(contentType, "+xml") {
		return ".xml"
	}
	return ".bin"
}
