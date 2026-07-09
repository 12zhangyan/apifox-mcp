package apifoxcli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return captureStdout(t, func() error {
		return run(args)
	})
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	runErr := fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), runErr
}

func TestCommandHelpDoesNotRequireRuntimeInputs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "validate endpoint",
			args: []string{"validate-endpoint", "--help"},
			want: "usage: apifox-cli validate-endpoint --file FILE",
		},
		{
			name: "api upsert",
			args: []string{"api", "upsert", "--help"},
			want: "preferred AI sync command",
		},
		{
			name: "apply docs",
			args: []string{"apply-docs", "--help"},
			want: "--batch-size 15 --dry-run",
		},
		{
			name: "schema create",
			args: []string{"schema", "create", "--help"},
			want: "usage: apifox-cli schema create --file FILE",
		},
		{
			name: "import openapi",
			args: []string{"import-openapi", "--help"},
			want: "usage: apifox-cli import-openapi",
		},
		{
			name: "legacy endpoint alias",
			args: []string{"upsert-endpoint", "--help"},
			want: "Legacy alias for apifox-cli api upsert",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := captureRun(t, tc.args...)
			if err != nil {
				t.Fatalf("expected help to return nil error, got %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("help output missing %q\noutput:\n%s", tc.want, out)
			}
		})
	}
}

func TestApplyDocsDryRunSelectsBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".apifox-docs.json")
	raw, err := json.Marshal(docsTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "apply-docs", "--file", path, "--dry-run", "--offset", "1", "--limit", "1")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output should be JSON: %v\n%s", err, out)
	}
	if got := int(payload["total_operations"].(float64)); got != 2 {
		t.Fatalf("expected total_operations=2, got %d", got)
	}
	if got := int(payload["selected_operations"].(float64)); got != 1 {
		t.Fatalf("expected selected_operations=1, got %d", got)
	}
	if got := int(payload["offset"].(float64)); got != 1 {
		t.Fatalf("expected offset=1, got %d", got)
	}
}

func TestApplyDocsDryRunBatchPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".apifox-docs.json")
	raw, err := json.Marshal(docsTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "apply-docs", "--file", path, "--dry-run", "--batch-size", "1")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run output should be JSON: %v\n%s", err, out)
	}
	batches := payload["batches"].([]any)
	if len(batches) != 2 {
		t.Fatalf("expected two batches, got %#v", batches)
	}
	first := batches[0].(map[string]any)
	if int(first["offset"].(float64)) != 0 || int(first["limit"].(float64)) != 1 {
		t.Fatalf("unexpected first batch: %#v", first)
	}
	if !strings.Contains(toString(first["command"]), "--offset 0 --limit 1") {
		t.Fatalf("batch command should be copyable, got %#v", first["command"])
	}
}

func TestEndpointListJSONIsStructured(t *testing.T) {
	openapi := map[string]any{
		"paths": map[string]any{
			"/orders": map[string]any{
				"get": map[string]any{
					"summary":     "获取订单列表",
					"operationId": "list_orders",
					"description": "获取订单列表",
					"tags":        []any{"订单管理"},
					"responses":   map[string]any{"200": map[string]any{"description": "成功"}},
				},
			},
		},
	}

	apis := collectAPIEndpointInfos(openapi, "")
	if len(apis) != 1 {
		t.Fatalf("expected one endpoint, got %d", len(apis))
	}
	payload := endpointListJSON(apis, limitedEndpoints(apis, 50), "", 50)
	if _, exists := payload["result"]; exists {
		t.Fatalf("structured endpoint JSON must not use result wrapper: %#v", payload)
	}
	if got := payload["total"]; got != 1 {
		t.Fatalf("expected total=1, got %#v", got)
	}
	endpoints := payload["endpoints"].([]map[string]any)
	if endpoints[0]["method"] != "GET" || endpoints[0]["path"] != "/orders" {
		t.Fatalf("unexpected endpoint payload: %#v", endpoints[0])
	}
}

func TestDiscoveryJSONCommandsUseStructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects/123/export-openapi" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fakeOpenAPI()); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	baseArgs := []string{"--token", "test-token", "--project-id", "123", "--base-url", server.URL}
	cases := []struct {
		name      string
		args      []string
		arrayKey  string
		boolKey   string
		noWrapper bool
	}{
		{name: "api list", args: []string{"api", "list", "--json"}, arrayKey: "endpoints", noWrapper: true},
		{name: "schema list", args: []string{"schema", "list", "--json"}, arrayKey: "schemas", noWrapper: true},
		{name: "tag list", args: []string{"tag", "list", "--json"}, arrayKey: "tags", noWrapper: true},
		{name: "audit consistency", args: []string{"audit", "consistency", "--json"}, arrayKey: "issues", boolKey: "valid", noWrapper: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := captureRun(t, append(baseArgs, tc.args...)...)
			if err != nil {
				t.Fatalf("expected command to succeed, got %v\n%s", err, out)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatalf("output should be JSON: %v\n%s", err, out)
			}
			if tc.noWrapper {
				if _, exists := payload["result"]; exists {
					t.Fatalf("structured output must not use result wrapper: %#v", payload)
				}
			}
			if _, exists := payload[tc.arrayKey]; !exists {
				t.Fatalf("payload missing %q: %#v", tc.arrayKey, payload)
			}
			if tc.boolKey != "" {
				if _, ok := payload[tc.boolKey].(bool); !ok {
					t.Fatalf("payload %q should be bool: %#v", tc.boolKey, payload)
				}
			}
		})
	}
}

func TestWriteCommandsJSONUseStructuredOutput(t *testing.T) {
	var importRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects/123/import-openapi" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		importRequests++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("request payload should be JSON: %v", err)
		}
		if strings.TrimSpace(toString(payload["input"])) == "" {
			t.Fatalf("import payload should include input: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"counters": map[string]any{
					"endpointCreated": 1,
					"endpointUpdated": 2,
					"schemaCreated":   3,
					"schemaUpdated":   4,
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	endpointFile := writeJSONFile(t, dir, ".apifox-endpoint.json", endpointTemplate("POST"))
	schemaFile := writeJSONFile(t, dir, ".apifox-schema.json", schemaTemplate())
	crudFile := writeJSONFile(t, dir, ".apifox-crud.json", crudTemplate())
	baseArgs := []string{"--token", "test-token", "--project-id", "123", "--base-url", server.URL}
	cases := []struct {
		name string
		args []string
		kind string
	}{
		{name: "api upsert", args: []string{"api", "upsert", "--file", endpointFile, "--json"}, kind: "endpoint"},
		{name: "schema create", args: []string{"schema", "create", "--file", schemaFile, "--json"}, kind: "schema"},
		{name: "generate crud", args: []string{"generate-crud", "--file", crudFile, "--json"}, kind: "crud"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := captureRun(t, append(baseArgs, tc.args...)...)
			if err != nil {
				t.Fatalf("expected command to succeed, got %v\n%s", err, out)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatalf("output should be JSON: %v\n%s", err, out)
			}
			if _, exists := payload["result"]; exists {
				t.Fatalf("write JSON must not use result wrapper: %#v", payload)
			}
			if payload["kind"] != tc.kind {
				t.Fatalf("expected kind %q, got %#v", tc.kind, payload["kind"])
			}
			if _, ok := payload["counters"].(map[string]any); !ok {
				t.Fatalf("payload should include counters: %#v", payload)
			}
			if _, ok := payload["import_result"].(map[string]any); !ok {
				t.Fatalf("payload should include import_result: %#v", payload)
			}
		})
	}
	if importRequests != len(cases) {
		t.Fatalf("expected %d import requests, got %d", len(cases), importRequests)
	}
}

func TestHTTPErrorIncludesMethodAndEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		if err := json.NewEncoder(w).Encode(map[string]any{"message": "quota exceeded"}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	endpointFile := writeJSONFile(t, dir, ".apifox-endpoint.json", endpointTemplate("POST"))
	out, err := captureRun(t,
		"--token", "test-token",
		"--project-id", "123",
		"--base-url", server.URL,
		"api", "upsert",
		"--file", endpointFile,
		"--json",
	)
	if err == nil {
		t.Fatalf("expected command to fail\n%s", out)
	}
	message := err.Error()
	if !strings.Contains(message, "HTTP 403 POST /projects/123/import-openapi: quota exceeded") {
		t.Fatalf("unexpected error message: %s", message)
	}
}

func fakeOpenAPI() map[string]any {
	return map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "测试项目", "version": "1.0.0"},
		"paths": map[string]any{
			"/orders": map[string]any{
				"get": map[string]any{
					"summary":     "获取订单列表",
					"operationId": "list_orders",
					"description": "获取订单列表",
					"tags":        []any{"订单管理"},
					"responses": map[string]any{
						"200": map[string]any{"description": "成功"},
					},
				},
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"Order": map[string]any{
					"type":        "object",
					"description": "订单",
					"properties": map[string]any{
						"id": map[string]any{"type": "integer", "description": "订单ID"},
					},
					"required": []any{"id"},
				},
			},
		},
	}
}

func writeJSONFile(t *testing.T, dir string, name string, data any) string {
	t.Helper()
	path := filepath.Join(dir, name)
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTemplatesValidate(t *testing.T) {
	cases := []struct {
		name   string
		errors []string
	}{
		{name: "endpoint POST", errors: validateEndpointSpec(endpointTemplate("POST"), false)},
		{name: "endpoint GET", errors: validateEndpointSpec(endpointTemplate("GET"), false)},
		{name: "crud", errors: validateCRUDSpec(crudTemplate())},
		{name: "schema", errors: validateSchemaSpec(schemaTemplate())},
		{name: "docs", errors: validateDocsSpec(docsTemplate())},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.errors) > 0 {
				t.Fatalf("template should validate, got errors: %v", tc.errors)
			}
		})
	}
}

func TestBuildOpenAPISpecPayloadContract(t *testing.T) {
	openapi := buildOpenAPISpec(endpointTemplate("POST"), false)
	paths := mustMap(t, openapi["paths"])
	pathItem := mustMap(t, paths["/orders"])
	operation := mustMap(t, pathItem["post"])

	if operation["summary"] != "创建订单" {
		t.Fatalf("unexpected summary: %#v", operation["summary"])
	}
	requestBody := mustMap(t, operation["requestBody"])
	content := mustMap(t, requestBody["content"])
	jsonContent := mustMap(t, content["application/json"])
	requestSchema := mustMap(t, jsonContent["schema"])
	if requestSchema["$ref"] != "#/components/schemas/CreateOrdersRequest" {
		t.Fatalf("unexpected request schema ref: %#v", requestSchema)
	}

	responses := mustMap(t, operation["responses"])
	success := mustMap(t, responses["200"])
	successContent := mustMap(t, success["content"])
	successJSON := mustMap(t, successContent["application/json"])
	successSchema := mustMap(t, successJSON["schema"])
	if successSchema["$ref"] != "#/components/schemas/CreateOrdersResponse" {
		t.Fatalf("unexpected response schema ref: %#v", successSchema)
	}
	if _, exists := responses["409"]; !exists {
		t.Fatalf("POST endpoint should include standard 409 response: %#v", responses)
	}
	errorResp := mustMap(t, responses["400"])
	errorContent := mustMap(t, errorResp["content"])
	errorJSON := mustMap(t, errorContent["application/json"])
	errorSchema := mustMap(t, errorJSON["schema"])
	if errorSchema["$ref"] != "#/components/schemas/ErrorResponse" {
		t.Fatalf("unexpected error schema ref: %#v", errorSchema)
	}
}

func TestBuildCRUDOpenAPIPayloadContract(t *testing.T) {
	openapi, created := buildCRUDOpenAPI(crudTemplate())
	wantCreated := []string{"GET /orders", "GET /orders/{id}", "POST /orders", "PUT /orders/{id}", "DELETE /orders/{id}"}
	if strings.Join(created, "|") != strings.Join(wantCreated, "|") {
		t.Fatalf("unexpected created endpoints:\nwant %v\ngot  %v", wantCreated, created)
	}

	paths := mustMap(t, openapi["paths"])
	collection := mustMap(t, paths["/orders"])
	if _, exists := collection["get"]; !exists {
		t.Fatalf("CRUD payload should include list operation: %#v", collection)
	}
	if _, exists := collection["post"]; !exists {
		t.Fatalf("CRUD payload should include create operation: %#v", collection)
	}
	detail := mustMap(t, paths["/orders/{id}"])
	for _, method := range []string{"get", "put", "delete"} {
		if _, exists := detail[method]; !exists {
			t.Fatalf("CRUD detail payload missing %s operation: %#v", method, detail)
		}
	}

	components := mustMap(t, openapi["components"])
	schemas := mustMap(t, components["schemas"])
	for _, name := range []string{"Order", "CreateOrderRequest", "OrderListResponse", "ErrorResponse"} {
		if _, exists := schemas[name]; !exists {
			t.Fatalf("CRUD payload missing schema %s: %#v", name, schemas)
		}
	}
}

func TestPrintValidationJSONUsesStableErrorsArray(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return printValidation(nil, true)
	})
	if err != nil {
		t.Fatalf("expected valid validation to return nil error, got %v", err)
	}
	if !strings.Contains(out, `"errors": []`) {
		t.Fatalf("expected empty errors array, got:\n%s", out)
	}
}

func TestValidateEndpointSpecFindsUnknownFieldsAndPlaceholders(t *testing.T) {
	spec := endpointTemplate("POST")
	spec["unexpected"] = true
	spec["response_example"].(map[string]any)["status"] = "string"

	errors := validateEndpointSpec(spec, false)
	if !containsError(errors, "unknown fields: unexpected") {
		t.Fatalf("expected unknown field error, got %v", errors)
	}
	if !containsError(errors, `response_example contains placeholder values: status="string"`) {
		t.Fatalf("expected placeholder error, got %v", errors)
	}
}

func TestDocsOperationsStripEndpointAction(t *testing.T) {
	endpoint := endpointTemplate("GET")
	endpoint["action"] = "upsert"
	docs := map[string]any{"endpoints": []any{endpoint}}

	if errors := validateDocsSpec(docs); len(errors) > 0 {
		t.Fatalf("docs spec with endpoint action should validate, got %v", errors)
	}

	operations := docsOperations(docs)
	if len(operations) != 1 {
		t.Fatalf("expected one operation, got %d", len(operations))
	}
	spec, ok := operations[0]["spec"].(map[string]any)
	if !ok {
		t.Fatalf("operation spec should be an object: %#v", operations[0]["spec"])
	}
	if _, exists := spec["action"]; exists {
		t.Fatalf("operation spec should not include action: %#v", spec)
	}
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %#v", value)
	}
	return obj
}

func containsError(errors []string, want string) bool {
	for _, errText := range errors {
		if strings.Contains(errText, want) {
			return true
		}
	}
	return false
}
