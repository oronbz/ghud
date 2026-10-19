package plugin

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	printer "golang.org/x/text/message"
)

//go:embed schema/*.json
var schemaFiles embed.FS

// SchemaBase is the URL every shipped schema's $id starts with.
const SchemaBase = "https://visor.dev/schema/"

var SchemaNames = []string{"manifest.json", "protocol.json"}

func Schema(name string) []byte {
	b, _ := schemaFiles.ReadFile("schema/" + name)
	return b
}

// Bundle is every shipped schema in one JSON Schema document, each under
// $defs by its name without ".json", as `visor plugin schema` prints it.
func Bundle() []byte {
	defs := map[string]json.RawMessage{}
	for _, name := range SchemaNames {
		defs[strings.TrimSuffix(name, ".json")] = Schema(name)
	}
	b, _ := json.MarshalIndent(struct {
		Schema      string                     `json:"$schema"`
		ID          string                     `json:"$id"`
		Title       string                     `json:"title"`
		Description string                     `json:"description"`
		Defs        map[string]json.RawMessage `json:"$defs"`
	}{
		Schema:      "https://json-schema.org/draft/2020-12/schema",
		ID:          SchemaBase + "visor.json",
		Title:       "visor plugin schemas, protocol " + strconv.Itoa(Protocol),
		Description: "$defs/manifest is visor-plugin.toml read as JSON. $defs/protocol is every message between visor and a Plugin's process; check a result against the $defs entry of protocol matching its request: initializeResult, refreshResult, choicesResult or invokeResult.",
		Defs:        defs,
	}, "", "  ")
	return append(b, '\n')
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

func validateJSON(ref string, src []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(src))
	if err != nil {
		return err
	}
	return Validate(ref, doc)
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
