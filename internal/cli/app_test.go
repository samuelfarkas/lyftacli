package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cliResult struct {
	code    int
	output  map[string]any
	problem map[string]any
}

func TestCapabilitiesDoesNotRequireToken(t *testing.T) {
	t.Parallel()
	result := runCLI(t, nil, "capabilities")
	if result.code != 0 || result.problem != nil {
		t.Fatalf("result = %#v", result)
	}
	if result.output["name"] != "lyfta" {
		t.Fatalf("output = %#v", result.output)
	}
	schemas := result.output["schemas"].(map[string]any)
	collection := schemas["collection_create"].(map[string]any)
	if collection["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("schema = %#v", collection)
	}
}

func TestCollectionDryRunDoesNotRequireToken(t *testing.T) {
	t.Parallel()
	result := runCLI(t, nil, "collections", "create", "--title", " Strength ", "--dry-run")
	if result.code != 0 {
		t.Fatalf("result = %#v", result)
	}
	body := result.output["body"].(map[string]any)
	collection := body["collection"].(map[string]any)
	if collection["title"] != "Strength" || result.output["dry_run"] != true {
		t.Fatalf("output = %#v", result.output)
	}
}

func TestTemplateWorkoutFileDryRun(t *testing.T) {
	t.Parallel()
	path := writeTempJSON(t, `{"title":"Push","exercises":[]}`)
	result := runCLI(t, nil, "templates", "create", "--collection-id", "42", "--workout", path, "--dry-run")
	if result.code != 0 {
		t.Fatalf("result = %#v", result)
	}
	body := result.output["body"].(map[string]any)
	if body["collectionId"] != json.Number("42") || body["workout"].(map[string]any)["title"] != "Push" {
		t.Fatalf("body = %#v", body)
	}
}

func TestNetworkCommandWithoutTokenIsStructuredError(t *testing.T) {
	t.Parallel()
	result := runCLI(t, nil, "workouts", "list")
	if result.code != 3 || result.output != nil || errorType(result) != "configuration_error" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPayloadCannotBeMixedWithFields(t *testing.T) {
	t.Parallel()
	path := writeTempJSON(t, `{"collection":{"title":"A"}}`)
	result := runCLI(t, nil, "collections", "create", "--payload", path, "--title", "B", "--dry-run")
	if result.code != 2 || errorType(result) != "validation_error" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGlobalFlagsWorkAfterNestedSubcommand(t *testing.T) {
	t.Parallel()
	result := runCLI(t, nil, "capabilities", "--pretty")
	if result.code != 0 || result.output["name"] != "lyfta" {
		t.Fatalf("result = %#v", result)
	}
}

func TestLongFlagAbbreviationsAreRejected(t *testing.T) {
	t.Parallel()
	result := runCLI(t, nil, "--max-retr", "0", "capabilities")
	if result.code != 2 || errorType(result) != "usage_error" {
		t.Fatalf("result = %#v", result)
	}
	errorObject := result.problem["error"].(map[string]any)
	if !strings.Contains(errorObject["message"].(string), "unknown flag") || errorObject["hint"] == nil {
		t.Fatalf("error = %#v", errorObject)
	}
}

func TestFullCollectionPayloadIsValidatedDuringDryRun(t *testing.T) {
	t.Parallel()
	path := writeTempJSON(t, `{}`)
	result := runCLI(t, nil, "collections", "create", "--payload", path, "--dry-run")
	if result.code != 2 || errorType(result) != "validation_error" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGetRequestRejectsBodyBeforeCredentials(t *testing.T) {
	t.Parallel()
	path := writeTempJSON(t, `{}`)
	result := runCLI(t, nil, "request", "GET", "/api/v1/workouts", "--data", path, "--dry-run")
	if result.code != 2 || errorType(result) != "validation_error" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMissingSubcommandIsUsageError(t *testing.T) {
	t.Parallel()
	result := runCLI(t, nil, "workouts")
	if result.code != 2 || errorType(result) != "usage_error" {
		t.Fatalf("result = %#v", result)
	}
}

func TestJSONInputRejectsTrailingData(t *testing.T) {
	t.Parallel()
	path := writeTempJSON(t, `{"collection":{"title":"A"}} {}`)
	result := runCLI(t, nil, "collections", "create", "--payload", path, "--dry-run")
	if result.code != 2 || errorType(result) != "input_error" {
		t.Fatalf("result = %#v", result)
	}
}

func runCLI(t *testing.T, environment map[string]string, arguments ...string) cliResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := New(bytes.NewReader(nil), &stdout, &stderr)
	app.getenv = func(key string) string { return environment[key] }
	result := cliResult{code: app.Execute(context.Background(), arguments)}
	result.output = decodeJSONStream(t, "stdout", &stdout)
	result.problem = decodeJSONStream(t, "stderr", &stderr)
	if result.code == 0 && result.problem != nil {
		t.Fatalf("successful command wrote an error: %#v", result.problem)
	}
	if result.code != 0 && result.output != nil {
		t.Fatalf("failed command wrote to stdout: %#v", result.output)
	}
	return result
}

func decodeJSONStream(t *testing.T, name string, stream *bytes.Buffer) map[string]any {
	t.Helper()
	if stream.Len() == 0 {
		return nil
	}
	var value map[string]any
	decoder := json.NewDecoder(stream)
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("%s is not JSON: %v", name, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("%s contains trailing output", name)
	}
	return value
}

func writeTempJSON(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func errorType(result cliResult) string {
	if result.problem == nil {
		return ""
	}
	errorObject, _ := result.problem["error"].(map[string]any)
	value, _ := errorObject["type"].(string)
	return value
}
