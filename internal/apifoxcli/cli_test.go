package apifoxcli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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
		{
			name: "tag replace batch",
			args: []string{"tag", "replace-batch", "--help"},
			want: "without importing OpenAPI",
		},
		{
			name: "folder delete empty",
			args: []string{"folder", "delete-empty", "--help"},
			want: "subtrees contain no endpoints",
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

func TestExportOpenAPIFileCompatibilityAliasMapsToOutput(t *testing.T) {
	opts, _, err := parseOptions(
		[]string{"--file", "backup.json"},
		map[string]bool{"file": true, "output": true},
		map[string]string{"file": "output"},
	)
	if err != nil {
		t.Fatalf("expected --file compatibility alias to parse, got %v", err)
	}
	if got := optString(opts, "output", ""); got != "backup.json" {
		t.Fatalf("expected output path backup.json, got %q", got)
	}
}

func TestExportOpenAPIFileCompatibilityAliasWritesFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects/123/export-openapi" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fakeOpenAPI()); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "export.json")
	_, err := captureRun(t,
		"--token", "test-token",
		"--project-id", "123",
		"--base-url", server.URL,
		"export-openapi",
		"--file", output,
	)
	if err != nil {
		t.Fatalf("expected export to succeed, got %v", err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("--file compatibility alias did not write output: %v", err)
	}
	var exported map[string]any
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatalf("exported file should contain JSON: %v", err)
	}
	if exported["openapi"] != "3.0.0" {
		t.Fatalf("unexpected exported document: %#v", exported)
	}
}

func TestConfigCheckJSONFailsWhenCredentialsAreMissing(t *testing.T) {
	t.Setenv("APIFOX_TOKEN", "")
	t.Setenv("APIFOX_PROJECT_ID", "")

	out, err := captureRun(t, "config", "check", "--json")
	if err == nil {
		t.Fatal("expected missing credentials to fail")
	}
	var commandErr commandError
	if !errors.As(err, &commandErr) || commandErr.Code != 2 {
		t.Fatalf("expected command error code 2, got %#v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("config check output should be JSON: %v\n%s", err, out)
	}
	if payload["configured"] != false || payload["connected"] != false {
		t.Fatalf("unexpected config status: %#v", payload)
	}
	errorInfo := payload["error"].(map[string]any)
	if errorInfo["code"] != "MISSING_CREDENTIALS" {
		t.Fatalf("unexpected error payload: %#v", errorInfo)
	}
}

func TestConfigCheckCountsEndpointsSeparatelyFromPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/projects/123/http-apis" {
			t.Fatalf("config check must use lightweight endpoint index, got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{
			map[string]any{"id": 1, "method": "GET", "path": "/orders"},
			map[string]any{"id": 2, "method": "POST", "path": "/orders"},
		}})
	}))
	defer server.Close()

	app := App{
		Config: Config{Token: "test-token", ProjectID: "123", BaseURL: server.URL},
		Client: server.Client(),
	}
	result, err := app.checkConfig()
	if err != nil {
		t.Fatalf("expected config check to succeed, got %v", err)
	}
	payload := result.JSON.(map[string]any)
	if payload["endpoint_count"] != 2 || payload["path_count"] != 1 {
		t.Fatalf("unexpected endpoint/path counts: %#v", payload)
	}
	if payload["check_scope"] != "connectivity" {
		t.Fatalf("unexpected config check scope: %#v", payload)
	}
}

func TestProjectOverviewUsesLightweightIndexAndMarksSchemasUnloaded(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/projects/123/http-apis" {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{
				map[string]any{"id": 1, "name": "List", "method": "GET", "path": "/orders", "tags": []any{"Orders"}},
				map[string]any{"id": 2, "name": "Create", "method": "POST", "path": "/orders", "tags": []any{"Orders"}},
			}})
			return
		}
		t.Fatalf("overview must not export OpenAPI, got %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	app := App{
		Config: Config{Token: "test-token", ProjectID: "123", BaseURL: server.URL},
		Client: server.Client(),
	}
	result, err := app.projectOverview(1)
	if err != nil {
		t.Fatalf("expected overview to succeed, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("overview should use one lightweight index call, got %d requests", requests)
	}
	payload := result.JSON.(map[string]any)
	counts := payload["counts"].(map[string]any)
	if counts["endpoints"] != nil || counts["paths"] != nil || counts["schemas"] != nil {
		t.Fatalf("unexpected overview counts: %#v", counts)
	}
	if counts["design_records"] != 2 || counts["design_paths"] != 1 {
		t.Fatalf("overview should expose lightweight design-index counts: %#v", counts)
	}
	if payload["schema_data_loaded"] != false {
		t.Fatalf("overview must mark schema data as unloaded: %#v", payload)
	}
	if payload["openapi_data_loaded"] != false {
		t.Fatalf("overview must mark OpenAPI counts as unloaded: %#v", payload)
	}
	samples := payload["samples"].(map[string]any)
	if len(samples["endpoints"].([]map[string]any)) != 1 || len(samples["schemas"].([]map[string]any)) != 0 {
		t.Fatalf("overview samples should respect limit: %#v", samples)
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
	if _, exists := endpoints[0]["description"]; exists {
		t.Fatalf("endpoint list must omit full descriptions: %#v", endpoints[0])
	}
}

func TestSchemaListRanksNamedSchemasBeforeEmptyAndNumericNames(t *testing.T) {
	openapi := map[string]any{"components": map[string]any{"schemas": map[string]any{
		"": map[string]any{"type": "object"}, "10": map[string]any{"type": "object"},
		"RegisterRequest": map[string]any{"type": "object"}, "Account": map[string]any{"type": "object"},
	}}}
	infos := collectSchemaInfos(openapi, "")
	if got := []string{infos[0].Name, infos[1].Name}; !reflect.DeepEqual(got, []string{"Account", "RegisterRequest"}) {
		t.Fatalf("useful schemas should sort first, got %v", got)
	}
	payload := schemaListJSON(infos, infos[:2], "", 2)
	if _, exists := payload["schemas"].([]map[string]any)[0]["description"]; exists {
		t.Fatalf("schema list must omit full descriptions: %#v", payload)
	}
}

func TestAPIGetUsesLightweightIndexWithoutOpenAPIExport(t *testing.T) {
	var indexCalls, exportCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/projects/123/http-apis":
			indexCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{map[string]any{
				"id": 11, "name": "Register", "method": "POST", "path": "/api/register", "tags": []any{"Auth"},
				"parameters": map[string]any{"header": []any{map[string]any{"name": "Cookie", "type": "string"}}},
				"responses": []any{map[string]any{
					"id": "response-200", "code": 200, "description": "OK", "contentType": "application/json",
					"jsonSchema": map[string]any{"type": "object"},
				}},
				"responseExamples": []any{map[string]any{
					"id": "example-1", "responseId": "response-200", "name": "Success",
					"data": `{"password":"plain","displayName":"Alice"}`,
				}},
			}}})
		case "/v1/projects/123/export-openapi":
			exportCalls++
			t.Fatal("api get must not export OpenAPI")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	app := App{Config: Config{Token: "test", ProjectID: "123", BaseURL: server.URL}, Client: server.Client(), ReadCacheTTL: time.Minute}
	first, err := app.getEndpointDetail("api/register", "post")
	if err != nil || first.JSON.(map[string]any)["found"] != true {
		t.Fatalf("expected endpoint detail, got %#v, %v", first.JSON, err)
	}
	operation := first.JSON.(map[string]any)["operation"].(map[string]any)
	response := operation["responses"].(map[string]any)["200"].(map[string]any)
	media := response["content"].(map[string]any)["application/json"].(map[string]any)
	examples := media["examples"].(map[string]any)
	if len(examples) != 1 {
		t.Fatalf("api get should retain response examples, got %#v", media)
	}
	value := examples["Success"].(map[string]any)["value"].(map[string]any)
	if value["password"] != "plain" || value["displayName"] != "Alice" {
		t.Fatalf("response example JSON should be decoded structurally, got %#v", value)
	}
	miss, err := app.getEndpointDetail("/missing", "GET")
	if err != nil || miss.JSON.(map[string]any)["found"] != false {
		t.Fatalf("expected fast miss, got %#v, %v", miss.JSON, err)
	}
	_, _ = app.getEndpointDetail("/api/register", "POST")
	if indexCalls != 1 || exportCalls != 0 {
		t.Fatalf("expected cached index/detail and no export for miss, index=%d export=%d", indexCalls, exportCalls)
	}
}

func TestPathNamingProjectStyleAcceptsCamelCaseAndReportsSuggestions(t *testing.T) {
	app := App{ReadCacheTTL: time.Minute, httpAPIRecords: []httpAPIRecord{
		{ID: 1, Method: "GET", Path: "/getCaptcha", Name: "Captcha", Detail: map[string]any{}},
		{ID: 2, Method: "POST", Path: "/Bad_Path", Name: "Bad", Detail: map[string]any{}},
	}, httpAPIRecordsAt: time.Now()}
	app.Config = Config{Token: "test", ProjectID: "123"}
	result, err := app.checkPathNaming("project")
	if err != nil {
		t.Fatal(err)
	}
	issues := result.JSON.(map[string]any)["issues"].([]map[string]any)
	if len(issues) != 1 || issues[0]["method"] != "POST" || issues[0]["expected"] != "badPath" {
		t.Fatalf("unexpected naming issues: %#v", issues)
	}
}

func TestUncategorizedTagRoundTripsFromTagListToTagAPIs(t *testing.T) {
	app := App{
		Config:       Config{Token: "test", ProjectID: "123"},
		ReadCacheTTL: time.Minute,
		httpAPIRecords: []httpAPIRecord{
			{ID: 1, Method: "GET", Path: "/untagged", Name: "Untagged", Detail: map[string]any{}},
			{ID: 2, Method: "GET", Path: "/tagged", Name: "Tagged", Tags: []string{"Orders"}, Detail: map[string]any{}},
		},
		httpAPIRecordsAt: time.Now(),
	}
	tagResult, err := app.listTags()
	if err != nil {
		t.Fatal(err)
	}
	tags := tagResult.JSON.(map[string]any)["tags"].([]map[string]any)
	if len(tags) != 2 {
		t.Fatalf("expected tagged and uncategorized groups, got %#v", tags)
	}
	apiResult, err := app.getAPIsByTag("未分类")
	if err != nil {
		t.Fatal(err)
	}
	payload := apiResult.JSON.(map[string]any)
	if payload["total"] != 1 {
		t.Fatalf("uncategorized tag should resolve to its endpoint, got %#v", payload)
	}
}

func TestDiscoveryJSONCommandsUseStructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/123/http-apis" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []any{map[string]any{
					"id": 11, "name": "获取订单列表", "method": "GET", "path": "/orders", "tags": []any{"订单管理"},
				}},
			})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects/123/export-openapi" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
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
	var importPayloads []map[string]any
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
		importPayloads = append(importPayloads, payload)
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
			if tc.kind == "endpoint" {
				if payload["persistence_verified"] != false || payload["read_back_verification_needed"] != true {
					t.Fatalf("endpoint result must require read-back verification: %#v", payload)
				}
			}
		})
	}
	if importRequests != len(cases) {
		t.Fatalf("expected %d import requests, got %d", len(cases), importRequests)
	}
	options := mustMap(t, importPayloads[0]["options"])
	if options["endpointOverwriteBehavior"] != "AUTO_MERGE" || options["schemaOverwriteBehavior"] != "AUTO_MERGE" {
		t.Fatalf("endpoint upsert must use non-destructive merge options: %#v", options)
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

func TestPartialEndpointUpdateOnlyEmitsProvidedFields(t *testing.T) {
	spec := map[string]any{
		"path":        "/orders/{id}",
		"method":      "POST",
		"description": "Updated business description",
	}
	if errors := validateEndpointSpec(spec, true); len(errors) > 0 {
		t.Fatalf("partial update should validate, got %v", errors)
	}

	openapi := buildOpenAPISpec(spec, true)
	operation := mustMap(t, mustMap(t, mustMap(t, openapi["paths"])["/orders/{id}"])["post"])
	if operation["description"] != "Updated business description" {
		t.Fatalf("unexpected description: %#v", operation)
	}
	for _, field := range []string{"summary", "tags", "requestBody", "responses"} {
		if _, exists := operation[field]; exists {
			t.Fatalf("partial update must not emit omitted field %q: %#v", field, operation)
		}
	}
	schemas := mustMap(t, mustMap(t, openapi["components"])["schemas"])
	if len(schemas) != 0 {
		t.Fatalf("description-only update must not create schemas: %#v", schemas)
	}
}

func TestPartialEndpointUpdateInlinesResponseSchema(t *testing.T) {
	spec := map[string]any{
		"path":   "/orders/{id}",
		"method": "GET",
		"response_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "Order ID"},
			},
		},
	}
	if errors := validateEndpointSpec(spec, true); len(errors) > 0 {
		t.Fatalf("partial response update should validate, got %v", errors)
	}
	openapi := buildOpenAPISpec(spec, true)
	operation := mustMap(t, mustMap(t, mustMap(t, openapi["paths"])["/orders/{id}"])["get"])
	responses := mustMap(t, operation["responses"])
	success := mustMap(t, responses["200"])
	content := mustMap(t, mustMap(t, success["content"])["application/json"])
	schema := mustMap(t, content["schema"])
	if _, isReference := schema["$ref"]; isReference {
		t.Fatalf("partial update should inline schema instead of creating a component: %#v", schema)
	}
}

func TestEndpointCookieParamsUseOpenAPICookieLocation(t *testing.T) {
	for _, field := range []string{"cookie_params", "cookies", "parameters"} {
		t.Run(field, func(t *testing.T) {
			parameterSpec := map[string]any{"name": "AuthenticationToken", "type": "string", "required": true, "description": "Authentication token"}
			if field == "parameters" {
				parameterSpec["in"] = "cookie"
				delete(parameterSpec, "type")
				parameterSpec["schema"] = map[string]any{"type": "string", "format": "token"}
			}
			spec := map[string]any{
				"path":   "/orders/{id}",
				"method": "GET",
				field:    []any{parameterSpec},
			}
			if errors := validateEndpointSpec(spec, true); len(errors) > 0 {
				t.Fatalf("cookie parameter update should validate, got %v", errors)
			}
			openapi := buildOpenAPISpec(spec, true)
			operation := mustMap(t, mustMap(t, mustMap(t, openapi["paths"])["/orders/{id}"])["get"])
			parameters, ok := toSlice(operation["parameters"])
			if !ok || len(parameters) != 1 {
				t.Fatalf("expected one cookie parameter, got %#v", operation["parameters"])
			}
			parameter := mustMap(t, parameters[0])
			if parameter["name"] != "AuthenticationToken" || parameter["in"] != "cookie" {
				t.Fatalf("unexpected cookie parameter: %#v", parameter)
			}
			if field == "parameters" && mustMap(t, parameter["schema"])["format"] != "token" {
				t.Fatalf("generic parameter schema was not preserved: %#v", parameter)
			}
		})
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
