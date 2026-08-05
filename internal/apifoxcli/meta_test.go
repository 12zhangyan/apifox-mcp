package apifoxcli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTagReplaceBatchUsesLightweightAPIAndSyncsFolder(t *testing.T) {
	var updatePayload map[string]any
	var importCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Apifox-Api-Version") != designAPIVersion || r.Header.Get("X-Project-Id") != "123" {
			t.Errorf("missing design API headers: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/123/http-apis":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []any{map[string]any{
					"id": 11, "name": "List orders", "method": "GET", "path": "/orders", "folderId": 1,
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/123/api-folders":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []any{map[string]any{
					"id": 1, "name": "EAM", "parentId": 0,
					"children": []any{map[string]any{"id": 2, "name": "Orders", "parentId": 1}},
				}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/123/http-apis/11":
			if err := json.NewDecoder(r.Body).Decode(&updatePayload); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 11}})
		case strings.Contains(r.URL.Path, "import-openapi"):
			importCalled = true
			http.Error(w, "import must not be called", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	input := writeJSONFile(t, t.TempDir(), "batch.json", map[string]any{
		"operations": []any{map[string]any{
			"method": "GET", "path": "/orders", "tags": []any{"Orders"}, "folder": "EAM/Orders",
		}},
	})
	out, err := captureRun(t,
		"--token", "test-token",
		"--project-id", "123",
		"--base-url", server.URL,
		"tag", "replace-batch", "--file", input, "--json",
	)
	if err != nil {
		t.Fatalf("expected tag replacement to succeed, got %v\n%s", err, out)
	}
	if importCalled {
		t.Fatal("tag replacement must not call import-openapi")
	}
	if !reflect.DeepEqual(updatePayload["tags"], []any{"Orders"}) {
		t.Fatalf("unexpected tags payload: %#v", updatePayload)
	}
	if toInt(updatePayload["folderId"], 0) != 2 {
		t.Fatalf("expected folderId 2, got %#v", updatePayload)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be JSON: %v\n%s", err, out)
	}
	counters := mustMap(t, result["counters"])
	if toInt(counters["updated"], 0) != 1 {
		t.Fatalf("unexpected counters: %#v", counters)
	}
}

func TestTagReplaceBatchDryRunDoesNotWrite(t *testing.T) {
	putCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			putCalls++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    []any{map[string]any{"id": 11, "method": "GET", "path": "/orders"}},
		})
	}))
	defer server.Close()

	input := writeJSONFile(t, t.TempDir(), "batch.json", map[string]any{
		"operations": []any{map[string]any{
			"method": "GET", "path": "/orders", "tags": []any{"Orders"},
		}},
	})
	out, err := captureRun(t,
		"--token", "test-token", "--project-id", "123", "--base-url", server.URL,
		"tag", "replace-batch", "--file", input, "--dry-run", "--json",
	)
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v\n%s", err, out)
	}
	if putCalls != 0 {
		t.Fatalf("dry-run made %d PUT request(s)", putCalls)
	}
	if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, `"endpoint_id": 11`) {
		t.Fatalf("dry-run preview is incomplete:\n%s", out)
	}
}

func TestListHTTPAPIRecordsFollowsPagination(t *testing.T) {
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pages = append(pages, r.URL.Query().Get("page"))
		if r.URL.Query().Get("perPage") != "100" {
			t.Errorf("unexpected perPage: %s", r.URL.Query().Get("perPage"))
		}
		if r.URL.Query().Get("page") == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    []any{map[string]any{"id": 11, "method": "GET", "path": "/first"}},
				"meta":    map[string]any{"page": 1, "nextPage": 2, "totalPages": 2},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    []any{map[string]any{"id": 12, "method": "POST", "path": "/second"}},
			"meta":    map[string]any{"page": 2, "totalPages": 2},
		})
	}))
	defer server.Close()

	app := &App{Config: Config{Token: "test-token", ProjectID: "123", BaseURL: server.URL}, Client: server.Client()}
	records, err := app.listHTTPAPIRecords()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pages, []string{"1", "2"}) {
		t.Fatalf("unexpected requested pages: %v", pages)
	}
	if len(records) != 2 || records[1].Path != "/second" {
		t.Fatalf("pagination did not return all records: %#v", records)
	}
}

func TestDeleteEmptyFoldersSkipsOccupiedTreeAndDeletesDeepestFirst(t *testing.T) {
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/123/api-folders":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []any{
					map[string]any{"id": 1, "name": "Used", "parentId": 0, "children": []any{
						map[string]any{"id": 2, "name": "Child", "parentId": 1, "children": []any{
							map[string]any{"id": 3, "name": "Leaf", "parentId": 2},
						}},
					}},
					map[string]any{"id": 4, "name": "Empty", "parentId": 0, "children": []any{
						map[string]any{"id": 5, "name": "Leaf", "parentId": 4},
					}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/123/http-apis":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    []any{map[string]any{"id": 11, "method": "GET", "path": "/orders", "folderId": 3}},
			})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	out, err := captureRun(t,
		"--token", "test-token", "--project-id", "123", "--base-url", server.URL,
		"folder", "delete-empty", "--all", "--confirm", "--json",
	)
	if err != nil {
		t.Fatalf("expected empty folder deletion to succeed, got %v\n%s", err, out)
	}
	want := []string{
		"/api/v1/projects/123/api-folders/5",
		"/api/v1/projects/123/api-folders/4",
	}
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("unexpected deletion order:\nwant %v\ngot  %v", want, deleted)
	}
}

func TestCallAPIRejectsBusinessFailureOnHTTP200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"code":    403,
			"message": "another user is importing",
		})
	}))
	defer server.Close()

	app := &App{Config: Config{Token: "test-token", ProjectID: "123", BaseURL: server.URL}, Client: server.Client()}
	_, err := app.callAPI(http.MethodPost, "/projects/123/import-openapi", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "another user is importing") {
		t.Fatalf("expected business failure, got %v", err)
	}
}

func TestExportOpenAPIIncludesApifoxFolderExtensionsByDefault(t *testing.T) {
	out, err := captureRun(t,
		"--token", "test-token", "--project-id", "123",
		"export-openapi", "--dry-run",
	)
	if err != nil {
		t.Fatalf("expected export dry-run to succeed, got %v\n%s", err, out)
	}
	if !strings.Contains(out, `"includeApifoxExtensionProperties": true`) {
		t.Fatalf("export should include Apifox extensions by default:\n%s", out)
	}

	out, err = captureRun(t,
		"--token", "test-token", "--project-id", "123",
		"export-openapi", "--exclude-apifox-extensions", "--dry-run",
	)
	if err != nil {
		t.Fatalf("expected portable export dry-run to succeed, got %v\n%s", err, out)
	}
	if !strings.Contains(out, `"includeApifoxExtensionProperties": false`) {
		t.Fatalf("exclude flag should disable Apifox extensions:\n%s", out)
	}
}
