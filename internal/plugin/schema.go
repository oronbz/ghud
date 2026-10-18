package plugin

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	printer "golang.org/x/text/message"
)

//go:embed schema/*.json
var schemaFiles embed.FS

// SchemaBase is the URL every shipped schema's $id starts with.
const SchemaBase = "https://ghud.dev/schema/"

// SchemaNames are the shipped schemas: the manifest and the protocol.
var SchemaNames = []string{"manifest.json", "protocol.json"}

// Schema is the shipped JSON Schema named name, one of SchemaNames.
func Schema(name string) []byte {
	b, _ := schemaFiles.ReadFile("schema/" + name)
	return b
}

var compiled = sync.OnceValues(func() (*jsonschema.Compiler, error) {
	c := jsonschema.NewCompiler()
	for _, name := range SchemaNames {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema(name)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := c.AddResource(SchemaBase+name, doc); err != nil {
			return nil, err
		}
	}
	return c, nil
})

// Validate checks instance, a value decoded from JSON or TOML, against the
// shipped schema at ref, such as "manifest.json" or
// "protocol.json#/$defs/invokeResult". The error names the first place it fails.
func Validate(ref string, instance any) error {
	c, err := compiled()
	if err != nil {
		return err
	}
	sch, err := c.Compile(SchemaBase + ref)
	if err != nil {
		return err
	}
	var ve *jsonschema.ValidationError
	if err := sch.Validate(instance); !errors.As(err, &ve) {
		return err
	}
	return errors.New(firstViolation(ve))
}

// firstViolation is the deepest leaf of a validation error, as `at: why`.
func firstViolation(ve *jsonschema.ValidationError) string {
	for len(ve.Causes) > 0 {
		ve = ve.Causes[0]
	}
	msg := ve.ErrorKind.LocalizedString(printer.NewPrinter(language.English))
	if at := path.Join(ve.InstanceLocation...); at != "" && at != "." {
		return strings.ReplaceAll(at, "/", ".") + ": " + msg
	}
	return msg
}
