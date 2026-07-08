package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const apiVersion = "2024-03-28"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	httpMethods      = set("GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS")
	requestBodyTypes = set("none", "json", "form-data", "x-www-form-urlencoded", "raw", "binary")
	crudOperations   = set("list", "get", "create", "update", "delete")
	schemaTypes      = set("object", "array", "string", "integer", "number", "boolean")

	endpointFields = set(
		"title", "path", "method", "description", "response_schema", "response_example",
		"folder_id", "status", "tags", "query_params", "path_params", "header_params",
		"request_body_type", "request_body_schema", "request_body_example", "responses",
	)
	endpointUpdateFields = set(
		"title", "path", "method", "description", "response_schema", "response_example",
		"folder_id", "status", "tags", "query_params", "path_params", "header_params",
		"request_body_type", "request_body_schema", "request_body_example", "responses",
		"new_path", "new_method",
	)
	crudFields = set(
		"resource_name", "resource_name_cn", "base_path", "model_schema", "id_field",
		"id_type", "operations", "tags", "folder_id", "description_prefix",
	)
	schemaFields = set(
		"name", "new_name", "schema_type", "type", "description", "properties", "required", "items", "folder_id",
	)
	docsFields          = set("endpoints", "crud")
	docsEndpointActions = set("create", "update", "upsert")

	pathParamRE      = regexp.MustCompile(`\{([^}/]+)\}`)
	pathNormalizeRE  = regexp.MustCompile(`\{[^}/]+\}`)
	splitNameRE      = regexp.MustCompile(`[-_/]+`)
	versionPathPart  = regexp.MustCompile(`^v[0-9]+$`)
	successCodeRE    = regexp.MustCompile(`^2[0-9][0-9]$`)
	kebabSegmentRE   = regexp.MustCompile(`^[a-z0-9{}]+(-[a-z0-9{}]+)*$`)
	snakeSegmentRE   = regexp.MustCompile(`^[a-z0-9{}]+(_[a-z0-9{}]+)*$`)
	camelSegmentRE   = regexp.MustCompile(`^[a-z][A-Za-z0-9{}]*$`)
	placeholderNames = set("", "string", "integer", "number", "boolean", "array", "object")
)

type Config struct {
	Token     string
	ProjectID string
	BaseURL   string
}

type App struct {
	Config Config
	Client *http.Client
}

type commandError struct {
	Message string
	Code    int
}

func (e commandError) Error() string { return e.Message }

func main() {
	if err := run(os.Args[1:]); err != nil {
		var ce commandError
		if errors.As(err, &ce) {
			if ce.Message != "" {
				fmt.Fprintf(os.Stderr, "error: %s\n", ce.Message)
			}
			os.Exit(ce.Code)
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	cfg, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 && (rest[0] == "--version" || rest[0] == "version") {
		fmt.Printf("apifox-mcp %s (%s, %s)\n", version, commit, date)
		return nil
	}
	if len(rest) == 0 || rest[0] == "-h" || rest[0] == "--help" {
		printHelp()
		return nil
	}

	app := &App{
		Config: cfg,
		Client: &http.Client{Timeout: 30 * time.Second},
	}

	command := rest[0]
	commandArgs := rest[1:]
	switch command {
	case "config":
		return app.cmdConfig(commandArgs)
	case "api":
		return app.cmdAPI(commandArgs)
	case "schema":
		return app.cmdSchema(commandArgs)
	case "tag":
		return app.cmdTag(commandArgs)
	case "folder":
		return app.cmdFolder(commandArgs)
	case "audit":
		return app.cmdAudit(commandArgs)
	case "endpoint-template":
		return app.cmdEndpointTemplate(commandArgs)
	case "validate-endpoint":
		return app.cmdValidateEndpoint(commandArgs)
	case "create-endpoint":
		return app.cmdWriteEndpoint(commandArgs, "create")
	case "update-endpoint":
		return app.cmdWriteEndpoint(commandArgs, "update")
	case "upsert-endpoint":
		return app.cmdWriteEndpoint(commandArgs, "upsert")
	case "crud-template":
		return app.cmdCRUDTemplate(commandArgs)
	case "validate-crud":
		return app.cmdValidateCRUD(commandArgs)
	case "generate-crud":
		return app.cmdGenerateCRUD(commandArgs)
	case "docs-template":
		return app.cmdDocsTemplate(commandArgs)
	case "validate-docs":
		return app.cmdValidateDocs(commandArgs)
	case "apply-docs":
		return app.cmdApplyDocs(commandArgs)
	case "versions":
		return app.cmdVersions(commandArgs)
	case "request":
		return app.cmdRequest(commandArgs)
	case "export-openapi":
		return app.cmdExportOpenAPI(commandArgs)
	case "import-openapi":
		return app.cmdImportOpenAPI(commandArgs)
	case "import-postman":
		return app.cmdImportPostman(commandArgs)
	default:
		return fail(2, "unknown command %q", command)
	}
}

func printHelp() {
	fmt.Println(`usage: apifox-mcp [--token TOKEN] [--project-id PROJECT_ID] [--base-url BASE_URL] <command> [options]

CLI for AI-authored Apifox/OpenAPI documentation.

Commands:
  version               Print CLI version
  config check          Check Apifox credentials and project connectivity
  api list              List HTTP endpoints
  api get               Show one HTTP endpoint
  api create            Create an endpoint from JSON spec
  api update            Update an endpoint from JSON spec
  api upsert            Create or update an endpoint from JSON spec
  api delete            Explain endpoint deletion support
  schema list           List schemas
  schema get            Show one schema
  schema create         Create a schema from JSON spec
  schema update         Update a schema from JSON spec
  schema delete         Explain schema deletion support
  tag list              List tags
  tag apis              List endpoints by tag
  tag add               Replace an endpoint's tags
  folder list           List folder/tag structure
  folder create         Explain folder creation support
  folder delete         Explain folder deletion support
  audit responses       Check one endpoint's response coverage
  audit all-responses   Audit response coverage across endpoints
  audit path-naming     Check path naming style
  audit consistency     Check response consistency
  docs-template         Print an AI-authorable batch docs JSON template
  validate-docs         Validate a batch docs JSON spec locally
  apply-docs            Apply endpoint and CRUD docs to Apifox
  endpoint-template     Print an endpoint JSON spec template
  validate-endpoint     Validate an endpoint JSON spec locally
  create-endpoint       Create an Apifox endpoint from JSON spec
  update-endpoint       Update an Apifox endpoint from JSON spec
  upsert-endpoint       Create or overwrite an Apifox endpoint from JSON spec
  crud-template         Print a CRUD JSON spec template
  validate-crud         Validate a CRUD JSON spec locally
  generate-crud         Generate CRUD endpoint docs in Apifox
  versions              List supported Apifox REST API versions
  request               Call a raw Apifox /v1 API endpoint
  export-openapi        Export OpenAPI/Swagger through official Apifox API
  import-openapi        Import OpenAPI/Swagger through official Apifox API
  import-postman        Import Postman Collection through official Apifox API`)
}

func parseGlobal(args []string) (Config, []string, error) {
	cfg := Config{
		Token:     os.Getenv("APIFOX_TOKEN"),
		ProjectID: os.Getenv("APIFOX_PROJECT_ID"),
		BaseURL:   strings.TrimRight(os.Getenv("APIFOX_BASE_URL"), "/"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.apifox.com"
	}

	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--token":
			if !hasValue {
				i++
				if i >= len(args) {
					return cfg, nil, fail(2, "--token requires a value")
				}
				value = args[i]
			}
			cfg.Token = value
		case "--project-id":
			if !hasValue {
				i++
				if i >= len(args) {
					return cfg, nil, fail(2, "--project-id requires a value")
				}
				value = args[i]
			}
			cfg.ProjectID = value
		case "--base-url":
			if !hasValue {
				i++
				if i >= len(args) {
					return cfg, nil, fail(2, "--base-url requires a value")
				}
				value = args[i]
			}
			cfg.BaseURL = strings.TrimRight(value, "/")
		default:
			rest = append(rest, arg)
		}
	}
	return cfg, rest, nil
}

func parseOptions(args []string, valueFlags map[string]bool, aliases map[string]string) (map[string][]string, []string, error) {
	opts := map[string][]string{}
	var pos []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			pos = append(pos, arg)
			continue
		}

		raw := strings.TrimLeft(arg, "-")
		name, value, hasValue := strings.Cut(raw, "=")
		if alias, ok := aliases[name]; ok {
			name = alias
		}
		takesValue := valueFlags[name]
		if takesValue {
			if !hasValue {
				i++
				if i >= len(args) {
					return nil, nil, fail(2, "--%s requires a value", name)
				}
				value = args[i]
			}
			opts[name] = append(opts[name], value)
			continue
		}
		if hasValue {
			opts[name] = append(opts[name], value)
		} else {
			opts[name] = append(opts[name], "true")
		}
	}
	return opts, pos, nil
}

func fail(code int, format string, args ...any) error {
	return commandError{Message: fmt.Sprintf(format, args...), Code: code}
}

func set(values ...string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func (a *App) requireToken() error {
	if a.Config.Token == "" {
		return fail(2, "缺少 APIFOX_TOKEN 环境变量，请设置 Apifox 访问令牌")
	}
	return nil
}

func (a *App) requireConfig() error {
	if a.Config.Token == "" {
		return fail(2, "缺少 APIFOX_TOKEN 环境变量，请设置 Apifox 访问令牌")
	}
	if a.Config.ProjectID == "" {
		return fail(2, "缺少 APIFOX_PROJECT_ID 环境变量，请设置目标项目 ID")
	}
	return nil
}

func (a *App) callAPI(method string, endpoint string, payload any, params url.Values) (any, error) {
	if err := a.requireToken(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(endpoint, "/") {
		return nil, fail(2, "endpoint must start with '/'")
	}
	u, err := url.Parse(a.Config.BaseURL + "/v1" + endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for key, values := range params {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	u.RawQuery = q.Encode()

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.Config.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Apifox-Api-Version", apiVersion)

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return nil, fail(1, "HTTP %d: %s", resp.StatusCode, extractError(raw))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil {
		return string(raw), nil
	}
	return data, nil
}

func extractError(raw []byte) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err == nil {
		for _, key := range []string{"message", "errorMessage", "error"} {
			if value := strings.TrimSpace(toString(obj[key])); value != "" {
				return value
			}
		}
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "未知错误"
	}
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

func readJSON(path string) (any, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

func readJSONObject(path string, label string) (map[string]any, error) {
	data, err := readJSON(path)
	if err != nil {
		return nil, err
	}
	obj, ok := data.(map[string]any)
	if !ok {
		return nil, fail(2, "%s must contain a JSON object", label)
	}
	return obj, nil
}

func readText(path string) (string, error) {
	if path == "-" {
		raw, err := io.ReadAll(os.Stdin)
		return string(raw), err
	}
	raw, err := os.ReadFile(path)
	return string(raw), err
}

func writeOutput(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func jsonPretty(data any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Sprintf("%v", data)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func jsonCompact(data any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func printJSON(data any) {
	fmt.Println(jsonPretty(data))
}

func optString(opts map[string][]string, key string, def string) string {
	values := opts[key]
	if len(values) == 0 {
		return def
	}
	return values[len(values)-1]
}

func optBool(opts map[string][]string, key string) bool {
	values := opts[key]
	if len(values) == 0 {
		return false
	}
	value := strings.ToLower(values[len(values)-1])
	return value == "true" || value == "1" || value == "yes"
}

func optList(opts map[string][]string, key string) []string {
	var out []string
	for _, value := range opts[key] {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func optIntList(opts map[string][]string, key string) ([]int, error) {
	var out []int
	for _, value := range optList(opts, key) {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, fail(2, "--%s must be an integer: %s", key, value)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func parseValue(raw string) any {
	if strings.HasPrefix(raw, "@") {
		data, err := readJSON(strings.TrimPrefix(raw, "@"))
		if err == nil {
			return data
		}
		return raw
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err == nil {
		return data
	}
	return raw
}

func parseJSONObject(raw string, label string) (map[string]any, error) {
	if strings.HasPrefix(raw, "@") {
		return readJSONObject(strings.TrimPrefix(raw, "@"), label)
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil {
		return nil, fail(2, "invalid JSON for %s: %s", label, err)
	}
	obj, ok := data.(map[string]any)
	if !ok {
		return nil, fail(2, "%s must be a JSON object", label)
	}
	return obj, nil
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func toInt(value any, def int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}
	return def
}

func toBool(value any, def bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
	}
	return def
}

func toMap(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func toSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unknownFields(spec map[string]any, allowed map[string]bool) []string {
	var out []string
	for key := range spec {
		if !allowed[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func checkSchemaDescriptions(schema map[string]any, path string) []string {
	properties, ok := toMap(schema["properties"])
	if !ok {
		return nil
	}
	var missing []string
	for name, raw := range properties {
		fullPath := name
		if path != "" {
			fullPath = path + "." + name
		}
		prop, ok := toMap(raw)
		if !ok {
			missing = append(missing, fullPath)
			continue
		}
		if strings.TrimSpace(toString(prop["description"])) == "" {
			missing = append(missing, fullPath)
		}
		if toString(prop["type"]) == "object" {
			missing = append(missing, checkSchemaDescriptions(prop, fullPath)...)
		}
		if toString(prop["type"]) == "array" {
			if items, ok := toMap(prop["items"]); ok && toString(items["type"]) == "object" {
				missing = append(missing, checkSchemaDescriptions(items, fullPath+"[]")...)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func findPlaceholderValues(value any, path string) []string {
	var placeholders []string
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			itemPath := key
			if path != "" {
				itemPath = path + "." + key
			}
			placeholders = append(placeholders, findPlaceholderValues(item, itemPath)...)
		}
	case []any:
		for index, item := range typed {
			itemPath := fmt.Sprintf("%s[%d]", path, index)
			placeholders = append(placeholders, findPlaceholderValues(item, itemPath)...)
		}
	case string:
		if placeholderNames[typed] {
			placeholders = append(placeholders, fmt.Sprintf("%s=%q", path, typed))
		}
	}
	return placeholders
}

func validateParamList(spec map[string]any, field string, errors *[]string) {
	raw, exists := spec[field]
	if !exists || raw == nil {
		return
	}
	params, ok := toSlice(raw)
	if !ok {
		*errors = append(*errors, field+" must be a list")
		return
	}
	for index, rawParam := range params {
		param, ok := toMap(rawParam)
		if !ok {
			*errors = append(*errors, fmt.Sprintf("%s[%d] must be an object", field, index))
			continue
		}
		if strings.TrimSpace(toString(param["name"])) == "" {
			*errors = append(*errors, fmt.Sprintf("%s[%d].name is required", field, index))
		}
		if strings.TrimSpace(toString(param["description"])) == "" {
			*errors = append(*errors, fmt.Sprintf("%s[%d].description is required", field, index))
		}
	}
}

func pathParamNames(path string) map[string]bool {
	out := map[string]bool{}
	for _, match := range pathParamRE.FindAllStringSubmatch(path, -1) {
		if len(match) > 1 {
			out[match[1]] = true
		}
	}
	return out
}

func normalizePathTemplate(path string) string {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	return pathNormalizeRE.ReplaceAllString(trimmed, "{}")
}

func validateEndpointSpec(spec map[string]any, update bool) []string {
	allowed := endpointFields
	if update {
		allowed = endpointUpdateFields
	}
	var errors []string
	if unknown := unknownFields(spec, allowed); len(unknown) > 0 {
		errors = append(errors, "unknown fields: "+strings.Join(unknown, ", "))
	}

	for _, field := range []string{"title", "path", "method", "description", "response_schema", "response_example"} {
		if isEmpty(spec[field]) {
			errors = append(errors, field+" is required")
		}
	}

	title := strings.TrimSpace(toString(spec["title"]))
	titleLower := strings.ToLower(title)
	for _, prefix := range []string{"get ", "post ", "put ", "delete ", "patch "} {
		if strings.HasPrefix(titleLower, prefix) {
			errors = append(errors, "title must be a Chinese business name, not an HTTP method phrase")
			break
		}
	}
	if strings.HasPrefix(title, "/") {
		errors = append(errors, "title must not be a path")
	}
	if strings.Contains(title, "-") || strings.Contains(title, "—") {
		errors = append(errors, "title must not include role prefixes separated by '-' or '—'")
	}

	path := toString(spec["path"])
	if update && strings.TrimSpace(toString(spec["new_path"])) != "" {
		path = toString(spec["new_path"])
	}
	if !strings.HasPrefix(path, "/") {
		errors = append(errors, "path must start with '/'")
	} else {
		names := pathParamNames(path)
		if _, exists := spec["path_params"]; exists {
			declared := map[string]bool{}
			if params, ok := toSlice(spec["path_params"]); ok {
				for _, rawParam := range params {
					if param, ok := toMap(rawParam); ok {
						name := toString(param["name"])
						if name != "" {
							declared[name] = true
						}
					}
				}
			}
			var missing, extra []string
			for name := range names {
				if !declared[name] {
					missing = append(missing, name)
				}
			}
			for name := range declared {
				if !names[name] {
					extra = append(extra, name)
				}
			}
			sort.Strings(missing)
			sort.Strings(extra)
			if len(missing) > 0 {
				errors = append(errors, "path_params missing definitions for: "+strings.Join(missing, ", "))
			}
			if len(extra) > 0 {
				errors = append(errors, "path_params contains names not present in path: "+strings.Join(extra, ", "))
			}
		} else if len(names) > 0 {
			var namesList []string
			for name := range names {
				namesList = append(namesList, name)
			}
			sort.Strings(namesList)
			errors = append(errors, "path_params is required when path contains placeholders: "+strings.Join(namesList, ", "))
		}
	}

	method := strings.ToUpper(toString(spec["method"]))
	if update && strings.TrimSpace(toString(spec["new_method"])) != "" {
		method = strings.ToUpper(toString(spec["new_method"]))
	}
	if !httpMethods[method] {
		errors = append(errors, "method must be one of: "+strings.Join(sortedKeys(httpMethods), ", "))
	}

	bodyType := toString(spec["request_body_type"])
	if bodyType == "" {
		bodyType = "json"
	}
	if !requestBodyTypes[bodyType] {
		errors = append(errors, "request_body_type must be one of: "+strings.Join(sortedKeys(requestBodyTypes), ", "))
	}

	responseSchema, ok := toMap(spec["response_schema"])
	if !ok {
		errors = append(errors, "response_schema must be an object")
	} else if missing := checkSchemaDescriptions(responseSchema, ""); len(missing) > 0 {
		errors = append(errors, "response_schema fields missing description: "+strings.Join(missing, ", "))
	}

	responseExample, ok := toMap(spec["response_example"])
	if !ok {
		errors = append(errors, "response_example must be an object")
	} else if placeholders := findPlaceholderValues(responseExample, ""); len(placeholders) > 0 {
		if len(placeholders) > 5 {
			placeholders = placeholders[:5]
		}
		errors = append(errors, "response_example contains placeholder values: "+strings.Join(placeholders, ", "))
	}

	if method == "POST" || method == "PUT" || method == "PATCH" {
		if isEmpty(spec["request_body_schema"]) {
			errors = append(errors, method+" requires request_body_schema")
		}
		if isEmpty(spec["request_body_example"]) {
			errors = append(errors, method+" requires request_body_example")
		}
	}

	if raw, exists := spec["request_body_schema"]; exists && raw != nil {
		bodySchema, ok := toMap(raw)
		if !ok {
			errors = append(errors, "request_body_schema must be an object")
		} else if missing := checkSchemaDescriptions(bodySchema, ""); len(missing) > 0 {
			errors = append(errors, "request_body_schema fields missing description: "+strings.Join(missing, ", "))
		}
	}
	if raw, exists := spec["request_body_example"]; exists && raw != nil {
		bodyExample, ok := toMap(raw)
		if !ok {
			errors = append(errors, "request_body_example must be an object")
		} else if placeholders := findPlaceholderValues(bodyExample, ""); len(placeholders) > 0 {
			if len(placeholders) > 5 {
				placeholders = placeholders[:5]
			}
			errors = append(errors, "request_body_example contains placeholder values: "+strings.Join(placeholders, ", "))
		}
	}

	for _, field := range []string{"query_params", "path_params", "header_params"} {
		validateParamList(spec, field, &errors)
	}
	if raw, exists := spec["tags"]; exists && raw != nil {
		if _, ok := toSlice(raw); !ok {
			errors = append(errors, "tags must be a list")
		}
	}
	if raw, exists := spec["responses"]; exists && raw != nil {
		if _, ok := toSlice(raw); !ok {
			errors = append(errors, "responses must be a list")
		}
	}
	return errors
}

func validateCRUDSpec(spec map[string]any) []string {
	var errors []string
	if unknown := unknownFields(spec, crudFields); len(unknown) > 0 {
		errors = append(errors, "unknown fields: "+strings.Join(unknown, ", "))
	}
	for _, field := range []string{"resource_name", "resource_name_cn", "base_path", "model_schema"} {
		if isEmpty(spec[field]) {
			errors = append(errors, field+" is required")
		}
	}
	if !strings.HasPrefix(toString(spec["base_path"]), "/") {
		errors = append(errors, "base_path must start with '/'")
	}
	modelSchema, ok := toMap(spec["model_schema"])
	if !ok {
		errors = append(errors, "model_schema must be an object")
	} else if properties, ok := toMap(modelSchema["properties"]); !ok || len(properties) == 0 {
		errors = append(errors, "model_schema.properties must be a non-empty object")
	} else if missing := checkSchemaDescriptions(modelSchema, ""); len(missing) > 0 {
		errors = append(errors, "model_schema fields missing description: "+strings.Join(missing, ", "))
	}
	if raw, exists := spec["operations"]; exists && raw != nil {
		ops, ok := toSlice(raw)
		if !ok {
			errors = append(errors, "operations must be a list")
		} else {
			var invalid []string
			for _, op := range ops {
				name := toString(op)
				if !crudOperations[name] {
					invalid = append(invalid, name)
				}
			}
			sort.Strings(invalid)
			if len(invalid) > 0 {
				errors = append(errors, "invalid operations: "+strings.Join(invalid, ", "))
			}
		}
	}
	if raw, exists := spec["tags"]; exists && raw != nil {
		if _, ok := toSlice(raw); !ok {
			errors = append(errors, "tags must be a list")
		}
	}
	return errors
}

func schemaType(spec map[string]any) string {
	value := toString(spec["schema_type"])
	if value == "" {
		value = toString(spec["type"])
	}
	if value == "" {
		value = "object"
	}
	return value
}

func validateSchemaSpec(spec map[string]any) []string {
	var errors []string
	if unknown := unknownFields(spec, schemaFields); len(unknown) > 0 {
		errors = append(errors, "unknown fields: "+strings.Join(unknown, ", "))
	}
	if isEmpty(spec["name"]) {
		errors = append(errors, "name is required")
	}
	typ := schemaType(spec)
	if !schemaTypes[typ] {
		errors = append(errors, "schema_type must be one of: "+strings.Join(sortedKeys(schemaTypes), ", "))
	}
	if strings.TrimSpace(toString(spec["description"])) == "" {
		errors = append(errors, "description is required")
	}
	if raw, exists := spec["properties"]; exists && raw != nil {
		properties, ok := toMap(raw)
		if !ok {
			errors = append(errors, "properties must be an object")
		} else if typ != "object" {
			errors = append(errors, "properties is only valid for object schemas")
		} else {
			schema := map[string]any{"type": "object", "properties": properties}
			if missing := checkSchemaDescriptions(schema, ""); len(missing) > 0 {
				errors = append(errors, "properties fields missing description: "+strings.Join(missing, ", "))
			}
		}
	}
	if typ == "object" && spec["properties"] == nil {
		errors = append(errors, "object schemas require properties")
	}
	if raw, exists := spec["required"]; exists && raw != nil {
		if _, ok := toSlice(raw); !ok {
			errors = append(errors, "required must be a list")
		}
	}
	if typ == "array" {
		items, ok := toMap(spec["items"])
		if !ok {
			errors = append(errors, "array schemas require items")
		} else if toString(items["type"]) == "object" {
			if missing := checkSchemaDescriptions(items, "items"); len(missing) > 0 {
				errors = append(errors, "items fields missing description: "+strings.Join(missing, ", "))
			}
		}
	}
	return errors
}

func stripEndpointAction(item map[string]any) (string, map[string]any) {
	action := toString(item["action"])
	if action == "" {
		action = "upsert"
	}
	spec := map[string]any{}
	for key, value := range item {
		if key != "action" {
			spec[key] = value
		}
	}
	return action, spec
}

func endpointDocKey(spec map[string]any, update bool) (string, bool) {
	path := toString(spec["path"])
	method := strings.ToUpper(toString(spec["method"]))
	if update && strings.TrimSpace(toString(spec["new_path"])) != "" {
		path = toString(spec["new_path"])
	}
	if update && strings.TrimSpace(toString(spec["new_method"])) != "" {
		method = strings.ToUpper(toString(spec["new_method"]))
	}
	if path == "" || !strings.HasPrefix(path, "/") || !httpMethods[method] {
		return "", false
	}
	return method + " " + normalizePathTemplate(path), true
}

func crudDocKeys(spec map[string]any) []string {
	basePath := toString(spec["base_path"])
	if !strings.HasPrefix(basePath, "/") {
		return nil
	}
	idField := toString(spec["id_field"])
	if idField == "" {
		idField = "id"
	}
	ops := []string{"list", "get", "create", "update", "delete"}
	if raw, exists := spec["operations"]; exists && raw != nil {
		if values, ok := toSlice(raw); ok {
			ops = nil
			for _, value := range values {
				ops = append(ops, toString(value))
			}
		}
	}
	detailPath := strings.TrimRight(basePath, "/") + "/{" + idField + "}"
	mapping := map[string]string{
		"list":   "GET " + normalizePathTemplate(basePath),
		"get":    "GET " + normalizePathTemplate(detailPath),
		"create": "POST " + normalizePathTemplate(basePath),
		"update": "PUT " + normalizePathTemplate(detailPath),
		"delete": "DELETE " + normalizePathTemplate(detailPath),
	}
	var keys []string
	for _, op := range ops {
		if key, ok := mapping[op]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func validateDocsSpec(spec map[string]any) []string {
	var errors []string
	var entries []struct {
		key    string
		source string
	}
	if unknown := unknownFields(spec, docsFields); len(unknown) > 0 {
		errors = append(errors, "unknown fields: "+strings.Join(unknown, ", "))
	}
	endpointsRaw := spec["endpoints"]
	crudRaw := spec["crud"]
	if isEmpty(endpointsRaw) && isEmpty(crudRaw) {
		errors = append(errors, "docs spec must include at least one endpoint or crud item")
	}
	if endpointsRaw == nil {
		endpointsRaw = []any{}
	}
	endpoints, ok := toSlice(endpointsRaw)
	if !ok {
		errors = append(errors, "endpoints must be a list")
	} else {
		for index, raw := range endpoints {
			item, ok := toMap(raw)
			if !ok {
				errors = append(errors, fmt.Sprintf("endpoints[%d] must be an object", index))
				continue
			}
			action, endpointSpec := stripEndpointAction(item)
			if !docsEndpointActions[action] {
				errors = append(errors, fmt.Sprintf("endpoints[%d].action must be one of: %s", index, strings.Join(sortedKeys(docsEndpointActions), ", ")))
			}
			update := action == "update" || action == "upsert"
			if key, ok := endpointDocKey(endpointSpec, update); ok {
				entries = append(entries, struct {
					key    string
					source string
				}{key: key, source: fmt.Sprintf("endpoints[%d]", index)})
			}
			for _, errText := range validateEndpointSpec(endpointSpec, update) {
				errors = append(errors, fmt.Sprintf("endpoints[%d]: %s", index, errText))
			}
		}
	}
	if crudRaw == nil {
		crudRaw = []any{}
	}
	crudSpecs, ok := toSlice(crudRaw)
	if !ok {
		errors = append(errors, "crud must be a list")
	} else {
		for index, raw := range crudSpecs {
			item, ok := toMap(raw)
			if !ok {
				errors = append(errors, fmt.Sprintf("crud[%d] must be an object", index))
				continue
			}
			for _, key := range crudDocKeys(item) {
				entries = append(entries, struct {
					key    string
					source string
				}{key: key, source: fmt.Sprintf("crud[%d]", index)})
			}
			for _, errText := range validateCRUDSpec(item) {
				errors = append(errors, fmt.Sprintf("crud[%d]: %s", index, errText))
			}
		}
	}
	seen := map[string]string{}
	for _, entry := range entries {
		if prev, exists := seen[entry.key]; exists {
			errors = append(errors, fmt.Sprintf("duplicate endpoint %s: %s and %s", entry.key, prev, entry.source))
		} else {
			seen[entry.key] = entry.source
		}
	}
	return errors
}

func printValidation(errors []string, asJSON bool) error {
	if asJSON {
		printJSON(map[string]any{"valid": len(errors) == 0, "errors": errors})
	} else if len(errors) > 0 {
		fmt.Println("spec 校验失败:")
		for _, errText := range errors {
			fmt.Println(" - " + errText)
		}
	} else {
		fmt.Println("spec 校验通过")
	}
	if len(errors) > 0 {
		return commandError{Code: 1}
	}
	return nil
}

func endpointTemplate(method string) map[string]any {
	method = strings.ToUpper(method)
	title := "获取订单详情"
	path := "/orders/{order_id}"
	if method == "POST" {
		title = "创建订单"
		path = "/orders"
	}
	template := map[string]any{
		"title":       title,
		"path":        path,
		"method":      method,
		"description": "接口业务说明，包含版本、鉴权和前置条件，不写请求响应示例",
		"tags":        []any{"订单管理"},
		"response_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"order_id": map[string]any{"type": "integer", "description": "订单ID"},
				"status":   map[string]any{"type": "string", "description": "订单状态"},
			},
			"required": []any{"order_id", "status"},
		},
		"response_example": map[string]any{"order_id": 90001, "status": "pending"},
	}
	if method == "GET" {
		template["path_params"] = []any{
			map[string]any{"name": "order_id", "type": "integer", "required": true, "description": "订单ID", "example": 90001},
		}
	}
	if method == "POST" || method == "PUT" || method == "PATCH" {
		template["request_body_schema"] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id":  map[string]any{"type": "integer", "description": "商品ID"},
				"quantity": map[string]any{"type": "integer", "description": "购买数量"},
			},
			"required": []any{"item_id", "quantity"},
		}
		template["request_body_example"] = map[string]any{"item_id": 1001, "quantity": 2}
	}
	return template
}

func crudTemplate() map[string]any {
	return map[string]any{
		"resource_name":    "order",
		"resource_name_cn": "订单",
		"base_path":        "/orders",
		"model_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "integer", "description": "订单ID"},
				"status":       map[string]any{"type": "string", "description": "订单状态"},
				"total_amount": map[string]any{"type": "number", "description": "订单总金额"},
			},
			"required": []any{"status", "total_amount"},
		},
		"operations": []any{"list", "get", "create", "update", "delete"},
		"tags":       []any{"订单管理"},
	}
}

func schemaTemplate() map[string]any {
	return map[string]any{
		"name":        "Order",
		"schema_type": "object",
		"description": "订单数据模型",
		"properties": map[string]any{
			"id":     map[string]any{"type": "integer", "description": "订单ID"},
			"status": map[string]any{"type": "string", "description": "订单状态"},
		},
		"required":  []any{"id", "status"},
		"folder_id": 0,
	}
}

func docsTemplate() map[string]any {
	return map[string]any{
		"endpoints": []any{
			map[string]any{
				"action":      "upsert",
				"title":       "获取订单统计",
				"path":        "/orders/statistics",
				"method":      "GET",
				"description": "获取订单数量和金额统计，需要用户已登录",
				"tags":        []any{"订单管理"},
				"query_params": []any{
					map[string]any{"name": "start_date", "type": "string", "required": false, "description": "统计开始日期，格式 YYYY-MM-DD", "example": "2026-07-01"},
					map[string]any{"name": "end_date", "type": "string", "required": false, "description": "统计结束日期，格式 YYYY-MM-DD", "example": "2026-07-31"},
				},
				"response_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"order_count":  map[string]any{"type": "integer", "description": "订单数量"},
						"total_amount": map[string]any{"type": "number", "description": "订单总金额"},
					},
					"required": []any{"order_count", "total_amount"},
				},
				"response_example": map[string]any{"order_count": 128, "total_amount": 93210.5},
			},
		},
		"crud": []any{crudTemplate()},
	}
}

func writeTemplate(data map[string]any, output string) error {
	text := jsonPretty(data) + "\n"
	if output != "" {
		if err := writeOutput(output, text); err != nil {
			return err
		}
		fmt.Printf("已写入 %s\n", output)
		return nil
	}
	fmt.Print(text)
	return nil
}

func (a *App) cmdEndpointTemplate(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"method": true, "output": true}, map[string]string{"o": "output"})
	if err != nil {
		return err
	}
	method := strings.ToUpper(optString(opts, "method", "POST"))
	if !httpMethods[method] {
		return fail(2, "method must be one of: %s", strings.Join(sortedKeys(httpMethods), ", "))
	}
	return writeTemplate(endpointTemplate(method), optString(opts, "output", ""))
}

func (a *App) cmdCRUDTemplate(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"output": true}, map[string]string{"o": "output"})
	if err != nil {
		return err
	}
	return writeTemplate(crudTemplate(), optString(opts, "output", ""))
}

func (a *App) cmdDocsTemplate(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"output": true}, map[string]string{"o": "output"})
	if err != nil {
		return err
	}
	return writeTemplate(docsTemplate(), optString(opts, "output", ""))
}

func (a *App) cmdValidateEndpoint(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"file": true}, nil)
	if err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	spec, err := readJSONObject(file, "--file")
	if err != nil {
		return err
	}
	return printValidation(validateEndpointSpec(spec, optBool(opts, "update")), optBool(opts, "json"))
}

func (a *App) cmdValidateCRUD(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"file": true}, nil)
	if err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	spec, err := readJSONObject(file, "--file")
	if err != nil {
		return err
	}
	return printValidation(validateCRUDSpec(spec), optBool(opts, "json"))
}

func (a *App) cmdValidateDocs(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"file": true}, nil)
	if err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	spec, err := readJSONObject(file, "--file")
	if err != nil {
		return err
	}
	return printValidation(validateDocsSpec(spec), optBool(opts, "json"))
}

func mustValid(errors []string, label string) error {
	if len(errors) == 0 {
		return nil
	}
	lines := []string{label + " 校验失败:"}
	for _, errText := range errors {
		lines = append(lines, "- "+errText)
	}
	return fail(2, strings.Join(lines, "\n"))
}

func optInt(opts map[string][]string, key string, def int) (int, error) {
	value := optString(opts, key, "")
	if value == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fail(2, "--%s must be an integer: %s", key, value)
	}
	return parsed, nil
}

func optOrPos(opts map[string][]string, key string, pos []string, index int) string {
	if value := optString(opts, key, ""); value != "" {
		return value
	}
	if index >= 0 && index < len(pos) {
		return pos[index]
	}
	return ""
}

func printTextResult(result string, opts map[string][]string) error {
	if optBool(opts, "json") {
		printJSON(map[string]any{"result": result})
		return nil
	}
	if file := optString(opts, "output", ""); file != "" {
		if err := writeOutput(file, result+"\n"); err != nil {
			return err
		}
		fmt.Printf("已写入 %s\n", file)
		return nil
	}
	fmt.Println(result)
	return nil
}

func (a *App) cmdConfig(args []string) error {
	if len(args) == 0 {
		args = []string{"check"}
	}
	switch args[0] {
	case "check":
		opts, _, err := parseOptions(args[1:], map[string]bool{"output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.checkConfig()
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "-h", "--help", "help":
		fmt.Println("usage: apifox-mcp config check [--json] [-o FILE]")
		return nil
	default:
		return fail(2, "unknown config command %q", args[0])
	}
}

func (a *App) cmdAPI(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`usage: apifox-mcp api <command> [options]

Commands:
  list                  List endpoints
  get                   Show one endpoint
  create                Create endpoint from --file
  update                Update endpoint from --file
  upsert                Create or update endpoint from --file
  delete                Explain endpoint deletion support`)
		return nil
	}
	switch args[0] {
	case "list":
		opts, _, err := parseOptions(args[1:], map[string]bool{"keyword": true, "limit": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		limit, err := optInt(opts, "limit", 50)
		if err != nil {
			return err
		}
		result, err := a.listAPIEndpoints(optString(opts, "keyword", ""), limit)
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "get":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"path": true, "method": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		method := strings.ToUpper(optOrPos(opts, "method", pos, 0))
		path := optOrPos(opts, "path", pos, 1)
		if strings.HasPrefix(method, "/") && path != "" {
			method, path = strings.ToUpper(path), method
		}
		result, err := a.getEndpointDetail(path, method)
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "create":
		return a.cmdWriteEndpoint(args[1:], "create")
	case "update":
		return a.cmdWriteEndpoint(args[1:], "update")
	case "upsert":
		return a.cmdWriteEndpoint(args[1:], "upsert")
	case "delete":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"path": true, "method": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		method := strings.ToUpper(optOrPos(opts, "method", pos, 0))
		path := optOrPos(opts, "path", pos, 1)
		if strings.HasPrefix(method, "/") && path != "" {
			method, path = strings.ToUpper(path), method
		}
		result, err := a.deleteEndpoint(path, method, optBool(opts, "confirm"))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	default:
		return fail(2, "unknown api command %q", args[0])
	}
}

func (a *App) cmdSchema(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`usage: apifox-mcp schema <command> [options]

Commands:
  template              Write a schema JSON template
  list                  List schemas
  get                   Show one schema
  create                Create schema from --file or flags
  update                Update schema from --file or flags
  delete                Explain schema deletion support`)
		return nil
	}
	switch args[0] {
	case "template":
		opts, _, err := parseOptions(args[1:], map[string]bool{"output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		return writeTemplate(schemaTemplate(), optString(opts, "output", ""))
	case "list":
		opts, _, err := parseOptions(args[1:], map[string]bool{"keyword": true, "limit": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		limit, err := optInt(opts, "limit", 50)
		if err != nil {
			return err
		}
		result, err := a.listSchemas(optString(opts, "keyword", ""), limit)
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "get":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"name": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.getSchemaDetail(optOrPos(opts, "name", pos, 0))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "create":
		return a.cmdWriteSchema(args[1:], "create")
	case "update":
		return a.cmdWriteSchema(args[1:], "update")
	case "delete":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"name": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.deleteSchema(optOrPos(opts, "name", pos, 0), optBool(opts, "confirm"))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	default:
		return fail(2, "unknown schema command %q", args[0])
	}
}

func (a *App) cmdTag(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`usage: apifox-mcp tag <command> [options]

Commands:
  list                  List tags
  apis                  List endpoints under one tag
  add                   Replace an endpoint's tags`)
		return nil
	}
	switch args[0] {
	case "list":
		opts, _, err := parseOptions(args[1:], map[string]bool{"output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.listTags()
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "apis":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"tag": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.getAPIsByTag(optOrPos(opts, "tag", pos, 0))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "add":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"path": true, "method": true, "tag": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		method := strings.ToUpper(optOrPos(opts, "method", pos, 0))
		path := optOrPos(opts, "path", pos, 1)
		if strings.HasPrefix(method, "/") && path != "" {
			method, path = strings.ToUpper(path), method
		}
		result, err := a.setEndpointTags(path, method, optList(opts, "tag"))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	default:
		return fail(2, "unknown tag command %q", args[0])
	}
}

func (a *App) cmdFolder(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`usage: apifox-mcp folder <command> [options]

Commands:
  list                  List folder/tag structure
  create                Explain folder creation support
  delete                Explain folder deletion support`)
		return nil
	}
	switch args[0] {
	case "list":
		opts, _, err := parseOptions(args[1:], map[string]bool{"output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.listFolders()
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "create":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"name": true, "description": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.createFolder(optOrPos(opts, "name", pos, 0), optString(opts, "description", ""))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "delete":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"name": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.deleteFolder(optOrPos(opts, "name", pos, 0), optBool(opts, "confirm"))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	default:
		return fail(2, "unknown folder command %q", args[0])
	}
}

func (a *App) cmdAudit(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`usage: apifox-mcp audit <command> [options]

Commands:
  responses             Check one endpoint's response coverage
  all-responses         Audit response coverage across endpoints
  path-naming           Check path naming style
  consistency           Check response consistency`)
		return nil
	}
	switch args[0] {
	case "responses":
		opts, pos, err := parseOptions(args[1:], map[string]bool{"path": true, "method": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		method := strings.ToUpper(optOrPos(opts, "method", pos, 0))
		path := optOrPos(opts, "path", pos, 1)
		if strings.HasPrefix(method, "/") && path != "" {
			method, path = strings.ToUpper(path), method
		}
		result, err := a.checkAPIResponses(path, method)
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "all-responses":
		opts, _, err := parseOptions(args[1:], map[string]bool{"tag": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.auditAllAPIResponses(optString(opts, "tag", ""), optBool(opts, "show-complete"))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "path-naming":
		opts, _, err := parseOptions(args[1:], map[string]bool{"style": true, "output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.checkPathNaming(optString(opts, "style", "kebab-case"))
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	case "consistency":
		opts, _, err := parseOptions(args[1:], map[string]bool{"output": true}, map[string]string{"o": "output"})
		if err != nil {
			return err
		}
		result, err := a.checkResponseConsistency()
		if err != nil {
			return err
		}
		return printTextResult(result, opts)
	default:
		return fail(2, "unknown audit command %q", args[0])
	}
}

func (a *App) cmdWriteEndpoint(args []string, action string) error {
	opts, _, err := parseOptions(args, map[string]bool{"file": true}, nil)
	if err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	spec, err := readJSONObject(file, "--file")
	if err != nil {
		return err
	}
	update := action == "update" || action == "upsert"
	if err := mustValid(validateEndpointSpec(spec, update), "spec"); err != nil {
		return err
	}
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printJSON(map[string]any{"command": "api " + action, "action": action, "spec": spec})
		return nil
	}
	result, err := a.applyEndpoint(spec, action)
	if err != nil {
		return err
	}
	if optBool(opts, "json") {
		printJSON(map[string]any{"result": result})
	} else {
		fmt.Println(result)
	}
	return nil
}

func schemaSpecFromOptions(opts map[string][]string) (map[string]any, error) {
	if file := optString(opts, "file", ""); file != "" {
		return readJSONObject(file, "--file")
	}
	spec := map[string]any{}
	for _, key := range []string{"name", "new-name", "schema-type", "type", "description"} {
		value := optString(opts, key, "")
		if value == "" {
			continue
		}
		switch key {
		case "new-name":
			spec["new_name"] = value
		case "schema-type":
			spec["schema_type"] = value
		default:
			spec[key] = value
		}
	}
	if value := optString(opts, "folder-id", ""); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil {
			return nil, fail(2, "--folder-id must be an integer")
		}
		spec["folder_id"] = id
	}
	if raw := optString(opts, "properties", ""); raw != "" {
		parsed := parseValue(raw)
		properties, ok := toMap(parsed)
		if !ok {
			return nil, fail(2, "--properties must be a JSON object or @file containing an object")
		}
		spec["properties"] = properties
	}
	if raw := optString(opts, "items", ""); raw != "" {
		parsed := parseValue(raw)
		items, ok := toMap(parsed)
		if !ok {
			return nil, fail(2, "--items must be a JSON object or @file containing an object")
		}
		spec["items"] = items
	}
	if required := optList(opts, "required"); len(required) > 0 {
		values := make([]any, 0, len(required))
		for _, value := range required {
			values = append(values, value)
		}
		spec["required"] = values
	}
	return spec, nil
}

func buildJSONSchema(spec map[string]any) map[string]any {
	schema := map[string]any{
		"type":        schemaType(spec),
		"description": toString(spec["description"]),
	}
	if properties, ok := toMap(spec["properties"]); ok {
		schema["properties"] = properties
	}
	if required, ok := toSlice(spec["required"]); ok && len(required) > 0 {
		schema["required"] = required
	}
	if items, ok := toMap(spec["items"]); ok {
		schema["items"] = items
	}
	return schema
}

func (a *App) cmdWriteSchema(args []string, action string) error {
	valueFlags := map[string]bool{
		"file": true, "name": true, "new-name": true, "schema-type": true, "type": true,
		"description": true, "properties": true, "items": true, "required": true, "folder-id": true,
	}
	opts, _, err := parseOptions(args, valueFlags, nil)
	if err != nil {
		return err
	}
	spec, err := schemaSpecFromOptions(opts)
	if err != nil {
		return err
	}
	if err := mustValid(validateSchemaSpec(spec), "schema spec"); err != nil {
		return err
	}
	command := "schema create"
	if action == "update" {
		command = "schema update"
	}
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printJSON(map[string]any{"command": command, "action": action, "spec": spec})
		return nil
	}
	result, err := a.applySchema(spec, action)
	if err != nil {
		return err
	}
	if optBool(opts, "json") {
		printJSON(map[string]any{"result": result})
	} else {
		fmt.Println(result)
	}
	return nil
}

func (a *App) cmdGenerateCRUD(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"file": true}, nil)
	if err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	spec, err := readJSONObject(file, "--file")
	if err != nil {
		return err
	}
	if err := mustValid(validateCRUDSpec(spec), "spec"); err != nil {
		return err
	}
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printJSON(map[string]any{"command": "generate-crud", "action": "generate", "spec": spec})
		return nil
	}
	result, err := a.applyCRUD(spec)
	if err != nil {
		return err
	}
	if optBool(opts, "json") {
		printJSON(map[string]any{"result": result})
	} else {
		fmt.Println(result)
	}
	return nil
}

func docsOperations(spec map[string]any) []map[string]any {
	var operations []map[string]any
	if endpoints, ok := toSlice(spec["endpoints"]); ok {
		for _, raw := range endpoints {
			item, ok := toMap(raw)
			if !ok {
				continue
			}
			action, endpointSpec := stripEndpointAction(item)
			operations = append(operations, map[string]any{"action": action, "kind": "endpoint", "command": "api " + action, "spec": endpointSpec})
		}
	}
	if crudSpecs, ok := toSlice(spec["crud"]); ok {
		for _, raw := range crudSpecs {
			item, ok := toMap(raw)
			if !ok {
				continue
			}
			operations = append(operations, map[string]any{"action": "generate", "kind": "crud", "command": "generate-crud", "spec": item})
		}
	}
	return operations
}

func (a *App) cmdApplyDocs(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"file": true}, nil)
	if err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	spec, err := readJSONObject(file, "--file")
	if err != nil {
		return err
	}
	if err := mustValid(validateDocsSpec(spec), "docs spec"); err != nil {
		return err
	}
	operations := docsOperations(spec)
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printJSON(map[string]any{"operations": operations})
		return nil
	}

	var results []map[string]any
	for index, operation := range operations {
		action := toString(operation["action"])
		kind := toString(operation["kind"])
		command := toString(operation["command"])
		argsMap, _ := toMap(operation["spec"])
		var result string
		var applyErr error
		if kind == "crud" {
			result, applyErr = a.applyCRUD(argsMap)
		} else {
			result, applyErr = a.applyEndpoint(argsMap, action)
		}
		item := map[string]any{"index": index + 1, "action": action, "kind": kind, "command": command, "result": result}
		results = append(results, item)
		if applyErr != nil {
			item["error"] = applyErr.Error()
			if optBool(opts, "json") {
				printJSON(map[string]any{"results": results})
			}
			return applyErr
		}
		if !optBool(opts, "json") {
			fmt.Printf("[%d/%d] %s\n%s\n", index+1, len(operations), command, result)
		}
	}
	if optBool(opts, "json") {
		printJSON(map[string]any{"results": results})
	}
	return nil
}

func standardErrorSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "标准错误响应",
		"properties": map[string]any{
			"code":    map[string]any{"type": "integer", "description": "错误码"},
			"message": map[string]any{"type": "string", "description": "错误信息"},
			"details": map[string]any{"type": "object", "description": "详细信息"},
		},
		"required": []any{"code", "message"},
	}
}

func standardErrorResponses() map[int]map[string]any {
	schema := standardErrorSchema()
	return map[int]map[string]any{
		400: {"code": 400, "name": "请求参数错误", "schema": schema, "example": map[string]any{"code": 400, "message": "请求参数错误", "details": map[string]any{"field": "name", "reason": "不能为空"}}},
		401: {"code": 401, "name": "未授权", "schema": schema, "example": map[string]any{"code": 401, "message": "未授权，请先登录"}},
		403: {"code": 403, "name": "禁止访问", "schema": schema, "example": map[string]any{"code": 403, "message": "无权限访问此资源"}},
		404: {"code": 404, "name": "资源不存在", "schema": schema, "example": map[string]any{"code": 404, "message": "请求的资源不存在"}},
		409: {"code": 409, "name": "资源冲突", "schema": schema, "example": map[string]any{"code": 409, "message": "资源已存在或状态冲突"}},
		422: {"code": 422, "name": "实体无法处理", "schema": schema, "example": map[string]any{"code": 422, "message": "请求格式正确但语义错误"}},
		500: {"code": 500, "name": "服务器内部错误", "schema": schema, "example": map[string]any{"code": 500, "message": "服务器内部错误，请稍后重试"}},
		502: {"code": 502, "name": "网关错误", "schema": schema, "example": map[string]any{"code": 502, "message": "网关错误，上游服务不可用"}},
		503: {"code": 503, "name": "服务不可用", "schema": schema, "example": map[string]any{"code": 503, "message": "服务暂时不可用，请稍后重试"}},
	}
}

func autoFillErrorResponses(rawResponses any, method string) []any {
	var responses []any
	if values, ok := toSlice(rawResponses); ok {
		responses = append(responses, values...)
	}
	existing := map[int]bool{}
	for _, raw := range responses {
		if resp, ok := toMap(raw); ok {
			existing[toInt(resp["code"], 0)] = true
		}
	}
	required := []int{400, 401, 403, 404, 500, 502, 503}
	if method == "POST" || method == "PUT" || method == "PATCH" {
		required = append(required, 409, 422)
	}
	standards := standardErrorResponses()
	for _, code := range required {
		if !existing[code] {
			responses = append(responses, standards[code])
		}
	}
	return responses
}

func toPascalCase(text string) string {
	text = strings.TrimLeft(text, "/")
	parts := splitNameRE.Split(text, -1)
	var out strings.Builder
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, "{") {
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			out.WriteString(part[1:])
		}
	}
	return out.String()
}

func generateSchemaName(path string, method string, suffix string, resourceName string) string {
	if resourceName == "" {
		var parts []string
		for _, part := range strings.Split(path, "/") {
			if part == "" || strings.HasPrefix(part, "{") || part == "api" || versionPathPart.MatchString(part) {
				continue
			}
			parts = append(parts, part)
		}
		if len(parts) > 0 {
			resourceName = toPascalCase(parts[len(parts)-1])
		} else {
			resourceName = "Resource"
		}
	}
	isSingle := strings.HasSuffix(strings.TrimRight(path, "/"), "}")
	prefix := ""
	switch strings.ToUpper(method) {
	case "POST":
		prefix = "Create"
	case "PUT", "PATCH":
		prefix = "Update"
	case "DELETE":
		prefix = "Delete"
	}
	if strings.ToUpper(method) == "GET" && !isSingle && suffix == "Response" {
		return resourceName + "ListResponse"
	}
	return prefix + resourceName + suffix
}

func buildOpenAPISpec(spec map[string]any, update bool) map[string]any {
	path := toString(spec["path"])
	method := strings.ToUpper(toString(spec["method"]))
	if update && strings.TrimSpace(toString(spec["new_path"])) != "" {
		path = toString(spec["new_path"])
	}
	if update && strings.TrimSpace(toString(spec["new_method"])) != "" {
		method = strings.ToUpper(toString(spec["new_method"]))
	}
	title := toString(spec["title"])
	description := toString(spec["description"])
	tags := spec["tags"]
	if tags == nil {
		tags = []any{}
	}

	components := map[string]any{"ErrorResponse": standardErrorSchema()}
	var parameters []any
	for _, pair := range []struct {
		field    string
		location string
	}{
		{"query_params", "query"},
		{"path_params", "path"},
		{"header_params", "header"},
	} {
		values, ok := toSlice(spec[pair.field])
		if !ok {
			continue
		}
		for _, rawParam := range values {
			param, ok := toMap(rawParam)
			if !ok {
				continue
			}
			required := toBool(param["required"], false)
			if pair.location == "path" {
				required = true
			}
			item := map[string]any{
				"name":        toString(param["name"]),
				"in":          pair.location,
				"required":    required,
				"description": toString(param["description"]),
				"schema":      map[string]any{"type": defaultString(toString(param["type"]), "string")},
			}
			if example, exists := param["example"]; exists {
				item["example"] = example
			}
			parameters = append(parameters, item)
		}
	}

	operation := map[string]any{
		"summary":     title,
		"description": description,
		"operationId": strings.ToLower(strings.ReplaceAll(title, " ", "_")),
		"tags":        tags,
	}
	if len(parameters) > 0 {
		operation["parameters"] = parameters
	}

	bodyType := defaultString(toString(spec["request_body_type"]), "json")
	if bodyType == "json" && (spec["request_body_schema"] != nil || spec["request_body_example"] != nil) {
		reqSchemaName := generateSchemaName(path, method, "Request", "")
		content := map[string]any{"application/json": map[string]any{}}
		jsonContent := content["application/json"].(map[string]any)
		if schema, ok := toMap(spec["request_body_schema"]); ok {
			components[reqSchemaName] = schema
			jsonContent["schema"] = map[string]any{"$ref": "#/components/schemas/" + reqSchemaName}
		}
		if example, exists := spec["request_body_example"]; exists {
			jsonContent["example"] = example
		}
		operation["requestBody"] = map[string]any{"required": true, "content": content}
	}

	methodForErrors := method
	responses := map[string]any{}
	responseItems := autoFillErrorResponses(spec["responses"], methodForErrors)
	success := map[string]any{"code": 200, "name": "成功", "schema": spec["response_schema"], "example": spec["response_example"]}
	var finalResponses []any
	finalResponses = append(finalResponses, success)
	for _, raw := range responseItems {
		if resp, ok := toMap(raw); ok && toInt(resp["code"], 0) != 200 {
			finalResponses = append(finalResponses, resp)
		}
	}
	for _, raw := range finalResponses {
		resp, ok := toMap(raw)
		if !ok {
			continue
		}
		codeInt := toInt(resp["code"], 200)
		code := strconv.Itoa(codeInt)
		respObj := map[string]any{"description": defaultString(toString(resp["name"]), "响应")}
		if resp["schema"] != nil || resp["example"] != nil {
			content := map[string]any{"application/json": map[string]any{}}
			jsonContent := content["application/json"].(map[string]any)
			if schema, ok := toMap(resp["schema"]); ok {
				if codeInt >= 400 {
					jsonContent["schema"] = map[string]any{"$ref": "#/components/schemas/ErrorResponse"}
				} else {
					respSchemaName := generateSchemaName(path, method, "Response", "")
					components[respSchemaName] = schema
					jsonContent["schema"] = map[string]any{"$ref": "#/components/schemas/" + respSchemaName}
				}
			}
			if example, exists := resp["example"]; exists {
				jsonContent["example"] = example
			}
			respObj["content"] = content
		}
		responses[code] = respObj
	}
	operation["responses"] = responses

	return map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": title, "version": "1.0.0"},
		"paths":   map[string]any{path: map[string]any{strings.ToLower(method): operation}},
		"components": map[string]any{
			"schemas": components,
		},
	}
}

func defaultString(value string, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

func (a *App) importOpenAPI(openapi map[string]any, endpointBehavior string, schemaBehavior string, folderID int) (any, error) {
	return a.importOpenAPIWithFolders(openapi, endpointBehavior, schemaBehavior, folderID, 0)
}

func (a *App) importOpenAPIWithFolders(openapi map[string]any, endpointBehavior string, schemaBehavior string, endpointFolderID int, schemaFolderID int) (any, error) {
	if err := a.requireConfig(); err != nil {
		return nil, err
	}
	input, err := jsonCompact(openapi)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"input": input,
		"options": map[string]any{
			"targetEndpointFolderId":    endpointFolderID,
			"targetSchemaFolderId":      schemaFolderID,
			"endpointOverwriteBehavior": endpointBehavior,
			"schemaOverwriteBehavior":   schemaBehavior,
		},
	}
	params := url.Values{"locale": []string{"zh-CN"}}
	return a.callAPI("POST", "/projects/"+a.Config.ProjectID+"/import-openapi", payload, params)
}

func (a *App) applyEndpoint(spec map[string]any, action string) (string, error) {
	if err := a.requireConfig(); err != nil {
		return "", err
	}
	update := action == "update" || action == "upsert"
	method := strings.ToUpper(toString(spec["method"]))
	path := toString(spec["path"])
	if action == "create" {
		exists, title, err := a.endpointExists(path, method)
		if err != nil {
			return "", err
		}
		if exists {
			return "", fail(1, "接口已存在，无法创建: %s %s (%s)", method, path, title)
		}
	}
	openapi := buildOpenAPISpec(spec, update)
	behavior := "OVERWRITE_EXISTING"
	if action == "create" {
		behavior = "CREATE_NEW"
	}
	result, err := a.importOpenAPI(openapi, behavior, "OVERWRITE_EXISTING", toInt(spec["folder_id"], 0))
	if err != nil {
		return "", err
	}
	counters := importCounters(result)
	finalPath := path
	finalMethod := method
	if update && strings.TrimSpace(toString(spec["new_path"])) != "" {
		finalPath = toString(spec["new_path"])
	}
	if update && strings.TrimSpace(toString(spec["new_method"])) != "" {
		finalMethod = strings.ToUpper(toString(spec["new_method"]))
	}
	return fmt.Sprintf("接口写入成功\n\n名称: %s\n路径: %s %s\n创建: %d\n更新: %d", toString(spec["title"]), finalMethod, finalPath, counters["endpointCreated"], counters["endpointUpdated"]), nil
}

func (a *App) applySchema(spec map[string]any, action string) (string, error) {
	if err := a.requireConfig(); err != nil {
		return "", err
	}
	name := toString(spec["name"])
	finalName := defaultString(toString(spec["new_name"]), name)
	openapi := map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "Schema: " + finalName, "version": "1.0.0"},
		"paths":   map[string]any{},
		"components": map[string]any{
			"schemas": map[string]any{finalName: buildJSONSchema(spec)},
		},
	}
	behavior := "CREATE_NEW"
	if action == "update" {
		behavior = "OVERWRITE_EXISTING"
	}
	result, err := a.importOpenAPIWithFolders(openapi, "OVERWRITE_EXISTING", behavior, 0, toInt(spec["folder_id"], 0))
	if err != nil {
		return "", err
	}
	counters := importCounters(result)
	actionName := "创建"
	if action == "update" {
		actionName = "更新"
	}
	return fmt.Sprintf("数据模型%s成功\n\n名称: %s\n类型: %s\n创建: %d\n更新: %d", actionName, finalName, schemaType(spec), counters["schemaCreated"], counters["schemaUpdated"]), nil
}

func importCounters(result any) map[string]int {
	out := map[string]int{"endpointCreated": 0, "endpointUpdated": 0, "schemaCreated": 0, "schemaUpdated": 0}
	obj, ok := toMap(result)
	if !ok {
		return out
	}
	data, ok := toMap(obj["data"])
	if !ok {
		return out
	}
	counters, ok := toMap(data["counters"])
	if !ok {
		return out
	}
	for key := range out {
		out[key] = toInt(counters[key], 0)
	}
	return out
}

func (a *App) endpointExists(path string, method string) (bool, string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return false, "", err
	}
	paths, ok := toMap(openapi["paths"])
	if !ok {
		return false, "", nil
	}
	pathItem, ok := toMap(paths[path])
	if !ok {
		return false, "", nil
	}
	operation, ok := toMap(pathItem[strings.ToLower(method)])
	if !ok {
		return false, "", nil
	}
	return true, toString(operation["summary"]), nil
}

func (a *App) exportOpenAPIMap(addFoldersToTags bool, includeExtensions bool) (map[string]any, error) {
	if err := a.requireConfig(); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"scope":        map[string]any{"type": "ALL"},
		"options":      map[string]any{"includeApifoxExtensionProperties": includeExtensions, "addFoldersToTags": addFoldersToTags},
		"oasVersion":   "3.1",
		"exportFormat": "JSON",
	}
	params := url.Values{"locale": []string{"zh-CN"}}
	result, err := a.callAPI("POST", "/projects/"+a.Config.ProjectID+"/export-openapi", payload, params)
	if err != nil {
		return nil, err
	}
	obj, ok := toMap(result)
	if !ok {
		return nil, fail(1, "Apifox export did not return a JSON object")
	}
	return obj, nil
}

func buildListSchema(itemSchemaOrRef map[string]any, resourceNameCN string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": resourceNameCN + "列表响应",
		"properties": map[string]any{
			"items":     map[string]any{"type": "array", "description": resourceNameCN + "列表", "items": itemSchemaOrRef},
			"total":     map[string]any{"type": "integer", "description": "总数量"},
			"page":      map[string]any{"type": "integer", "description": "当前页码"},
			"page_size": map[string]any{"type": "integer", "description": "每页数量"},
		},
		"required": []any{"items", "total"},
	}
}

func generateItemExample(schema map[string]any, idValue int) map[string]any {
	example := map[string]any{}
	properties, _ := toMap(schema["properties"])
	for name, raw := range properties {
		prop, _ := toMap(raw)
		propType := defaultString(toString(prop["type"]), "string")
		description := defaultString(toString(prop["description"]), name)
		switch {
		case name == "id":
			example[name] = idValue
		case propType == "integer":
			example[name] = 1
		case propType == "number":
			example[name] = 1.0
		case propType == "boolean":
			example[name] = true
		case propType == "array":
			example[name] = []any{}
		case propType == "object":
			example[name] = map[string]any{}
		case strings.Contains(strings.ToLower(name), "email"):
			example[name] = "user@example.com"
		case strings.Contains(strings.ToLower(name), "phone"):
			example[name] = "13800138000"
		case strings.Contains(strings.ToLower(name), "name"):
			example[name] = "示例名称"
		case strings.Contains(strings.ToLower(name), "time") || strings.Contains(strings.ToLower(name), "date"):
			example[name] = "2026-07-01T12:00:00Z"
		case strings.Contains(strings.ToLower(name), "url"):
			example[name] = "https://example.com"
		default:
			example[name] = "示例" + description
		}
	}
	return example
}

func buildResponsesWithRef(code int, name string, schemaName string, example any, method string, errorSchemaName string) map[string]any {
	responses := map[string]any{}
	resp := map[string]any{"description": name}
	if schemaName != "" {
		content := map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schemaName}}}
		if example != nil {
			content["application/json"].(map[string]any)["example"] = example
		}
		resp["content"] = content
	}
	responses[strconv.Itoa(code)] = resp
	for _, errResp := range autoFillErrorResponses(nil, method) {
		item, _ := toMap(errResp)
		errCode := strconv.Itoa(toInt(item["code"], 0))
		responses[errCode] = map[string]any{
			"description": item["name"],
			"content": map[string]any{
				"application/json": map[string]any{
					"schema":  map[string]any{"$ref": "#/components/schemas/" + errorSchemaName},
					"example": item["example"],
				},
			},
		}
	}
	return responses
}

func buildCRUDOpenAPI(spec map[string]any) (map[string]any, []string) {
	resourceName := toString(spec["resource_name"])
	resourceNameCN := toString(spec["resource_name_cn"])
	basePath := strings.TrimRight(toString(spec["base_path"]), "/")
	modelSchema, _ := toMap(spec["model_schema"])
	idField := defaultString(toString(spec["id_field"]), "id")
	idType := defaultString(toString(spec["id_type"]), "integer")
	descriptionPrefix := toString(spec["description_prefix"])
	tags := spec["tags"]
	if tags == nil {
		tags = []any{resourceNameCN + "管理"}
	}
	ops := set("list", "get", "create", "update", "delete")
	if raw, exists := spec["operations"]; exists && raw != nil {
		ops = map[string]bool{}
		if values, ok := toSlice(raw); ok {
			for _, value := range values {
				ops[toString(value)] = true
			}
		}
	}
	makeDesc := func(action string) string {
		desc := action + resourceNameCN
		if descriptionPrefix != "" {
			desc = descriptionPrefix + "\n\n" + desc
		}
		return desc
	}

	itemExample := generateItemExample(modelSchema, 1)
	properties, _ := toMap(modelSchema["properties"])
	createProperties := map[string]any{}
	for key, value := range properties {
		if key != idField {
			createProperties[key] = value
		}
	}
	var createRequired []any
	if required, ok := toSlice(modelSchema["required"]); ok {
		for _, value := range required {
			if toString(value) != idField {
				createRequired = append(createRequired, value)
			}
		}
	}
	createSchema := map[string]any{
		"type":        "object",
		"description": "创建" + resourceNameCN + "请求体",
		"properties":  createProperties,
		"required":    createRequired,
	}
	createExample := map[string]any{}
	for key, value := range itemExample {
		if key != idField {
			createExample[key] = value
		}
	}

	resourceSchemaName := toPascalCase(resourceName)
	createRequestSchemaName := "Create" + resourceSchemaName + "Request"
	listResponseSchemaName := resourceSchemaName + "ListResponse"
	errorSchemaName := "ErrorResponse"
	components := map[string]any{
		resourceSchemaName:      modelSchema,
		createRequestSchemaName: createSchema,
		listResponseSchemaName:  buildListSchema(map[string]any{"$ref": "#/components/schemas/" + resourceSchemaName}, resourceNameCN),
		errorSchemaName:         standardErrorSchema(),
	}

	paths := map[string]any{}
	var created []string
	if ops["list"] {
		paths[basePath] = mergePathItem(paths[basePath], "get", map[string]any{
			"summary":     "获取" + resourceNameCN + "列表",
			"description": makeDesc("获取") + "列表，支持分页",
			"operationId": "list_" + resourceName + "s",
			"tags":        tags,
			"parameters": []any{
				map[string]any{"name": "page", "in": "query", "required": false, "description": "页码", "schema": map[string]any{"type": "integer", "default": 1}},
				map[string]any{"name": "page_size", "in": "query", "required": false, "description": "每页数量", "schema": map[string]any{"type": "integer", "default": 20}},
			},
			"responses": buildResponsesWithRef(200, "成功", listResponseSchemaName, map[string]any{"items": []any{itemExample}, "total": 100, "page": 1, "page_size": 20}, "GET", errorSchemaName),
		})
		created = append(created, "GET "+basePath)
	}
	detailPath := basePath + "/{" + idField + "}"
	if ops["get"] {
		paths[detailPath] = mergePathItem(paths[detailPath], "get", map[string]any{
			"summary":     "获取" + resourceNameCN + "详情",
			"description": makeDesc("获取") + "详情",
			"operationId": "get_" + resourceName,
			"tags":        tags,
			"parameters":  []any{map[string]any{"name": idField, "in": "path", "required": true, "description": resourceNameCN + "ID", "schema": map[string]any{"type": idType}}},
			"responses":   buildResponsesWithRef(200, "成功", resourceSchemaName, itemExample, "GET", errorSchemaName),
		})
		created = append(created, "GET "+detailPath)
	}
	if ops["create"] {
		paths[basePath] = mergePathItem(paths[basePath], "post", map[string]any{
			"summary":     "创建" + resourceNameCN,
			"description": makeDesc("创建"),
			"operationId": "create_" + resourceName,
			"tags":        tags,
			"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + createRequestSchemaName}, "example": createExample}}},
			"responses":   buildResponsesWithRef(201, "创建成功", resourceSchemaName, itemExample, "POST", errorSchemaName),
		})
		created = append(created, "POST "+basePath)
	}
	if ops["update"] {
		paths[detailPath] = mergePathItem(paths[detailPath], "put", map[string]any{
			"summary":     "更新" + resourceNameCN,
			"description": makeDesc("更新"),
			"operationId": "update_" + resourceName,
			"tags":        tags,
			"parameters":  []any{map[string]any{"name": idField, "in": "path", "required": true, "description": resourceNameCN + "ID", "schema": map[string]any{"type": idType}}},
			"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + createRequestSchemaName}, "example": createExample}}},
			"responses":   buildResponsesWithRef(200, "更新成功", resourceSchemaName, itemExample, "PUT", errorSchemaName),
		})
		created = append(created, "PUT "+detailPath)
	}
	if ops["delete"] {
		paths[detailPath] = mergePathItem(paths[detailPath], "delete", map[string]any{
			"summary":     "删除" + resourceNameCN,
			"description": makeDesc("删除"),
			"operationId": "delete_" + resourceName,
			"tags":        tags,
			"parameters":  []any{map[string]any{"name": idField, "in": "path", "required": true, "description": resourceNameCN + "ID", "schema": map[string]any{"type": idType}}},
			"responses":   buildResponsesWithRef(204, "删除成功", "", nil, "DELETE", errorSchemaName),
		})
		created = append(created, "DELETE "+detailPath)
	}
	return map[string]any{
		"openapi":    "3.0.0",
		"info":       map[string]any{"title": resourceNameCN + " CRUD API", "version": "1.0.0"},
		"paths":      paths,
		"components": map[string]any{"schemas": components},
	}, created
}

func mergePathItem(raw any, method string, operation map[string]any) map[string]any {
	item, ok := toMap(raw)
	if !ok {
		item = map[string]any{}
	}
	item[method] = operation
	return item
}

func (a *App) applyCRUD(spec map[string]any) (string, error) {
	openapi, created := buildCRUDOpenAPI(spec)
	result, err := a.importOpenAPI(openapi, "OVERWRITE_EXISTING", "OVERWRITE_EXISTING", toInt(spec["folder_id"], 0))
	if err != nil {
		return "", err
	}
	counters := importCounters(result)
	return fmt.Sprintf("CRUD 接口批量写入成功\n\n资源: %s (%s)\n基础路径: %s\n创建: %d\n更新: %d\n接口:\n  %s",
		toString(spec["resource_name_cn"]),
		toString(spec["resource_name"]),
		toString(spec["base_path"]),
		counters["endpointCreated"],
		counters["endpointUpdated"],
		strings.Join(created, "\n  "),
	), nil
}

func (a *App) cmdExportOpenAPI(args []string) error {
	valueFlags := map[string]bool{"format": true, "oas-version": true, "scope": true, "endpoint-id": true, "tag": true, "folder-id": true, "exclude-tag": true, "environment-id": true, "branch-id": true, "module-id": true, "output": true, "locale": true}
	opts, _, err := parseOptions(args, valueFlags, map[string]string{"o": "output"})
	if err != nil {
		return err
	}
	if err := a.requireConfig(); err != nil {
		return err
	}
	format := strings.ToUpper(optString(opts, "format", "JSON"))
	if format != "JSON" && format != "YAML" {
		return fail(2, "--format must be JSON or YAML")
	}
	scope, err := buildExportScope(opts)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"scope":        scope,
		"options":      map[string]any{"includeApifoxExtensionProperties": optBool(opts, "include-apifox-extensions"), "addFoldersToTags": optBool(opts, "add-folders-to-tags")},
		"oasVersion":   optString(opts, "oas-version", "3.1"),
		"exportFormat": format,
	}
	if ids, err := optIntList(opts, "environment-id"); err != nil {
		return err
	} else if len(ids) > 0 {
		payload["environmentIds"] = ids
	}
	if value := optString(opts, "branch-id", ""); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil {
			return fail(2, "--branch-id must be an integer")
		}
		payload["branchId"] = id
	}
	if value := optString(opts, "module-id", ""); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil {
			return fail(2, "--module-id must be an integer")
		}
		payload["moduleId"] = id
	}
	endpoint := "/projects/" + a.Config.ProjectID + "/export-openapi"
	locale := optString(opts, "locale", "zh-CN")
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printRequestPreview("POST", endpoint, payload, nil, locale)
		return nil
	}
	result, err := a.callAPI("POST", endpoint, payload, url.Values{"locale": []string{locale}})
	if err != nil {
		return err
	}
	output := normalizeExportPayload(result, format)
	if file := optString(opts, "output", ""); file != "" {
		if err := writeOutput(file, output); err != nil {
			return err
		}
		fmt.Printf("已导出 %s 到 %s\n", format, file)
		return nil
	}
	fmt.Println(output)
	return nil
}

func buildExportScope(opts map[string][]string) (map[string]any, error) {
	scopeName := optString(opts, "scope", "all")
	excluded := optList(opts, "exclude-tag")
	var scope map[string]any
	switch scopeName {
	case "all":
		scope = map[string]any{"type": "ALL"}
	case "endpoints":
		ids, err := optIntList(opts, "endpoint-id")
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, fail(2, "--scope endpoints requires at least one --endpoint-id")
		}
		scope = map[string]any{"type": "SELECTED_ENDPOINTS", "selectedEndpointIds": ids}
	case "tags":
		tags := optList(opts, "tag")
		if len(tags) == 0 {
			return nil, fail(2, "--scope tags requires at least one --tag")
		}
		scope = map[string]any{"type": "SELECTED_TAGS", "selectedTags": tags}
	case "folders":
		ids, err := optIntList(opts, "folder-id")
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, fail(2, "--scope folders requires at least one --folder-id")
		}
		scope = map[string]any{"type": "SELECTED_FOLDERS", "selectedFolderIds": ids}
	default:
		return nil, fail(2, "unsupported export scope: %s", scopeName)
	}
	if len(excluded) > 0 {
		scope["excludedByTags"] = excluded
	}
	return scope, nil
}

func normalizeExportPayload(result any, format string) string {
	if text, ok := result.(string); ok {
		return text
	}
	if obj, ok := toMap(result); ok && len(obj) == 1 {
		if raw, exists := obj["raw"]; exists {
			return toString(raw)
		}
	}
	return jsonPretty(result)
}

func importOptions(opts map[string][]string, includeSchema bool) map[string]any {
	options := map[string]any{
		"endpointOverwriteBehavior":     optString(opts, "endpoint-overwrite-behavior", "OVERWRITE_EXISTING"),
		"updateFolderOfChangedEndpoint": optBool(opts, "update-folder-of-changed-endpoint"),
	}
	if value := optString(opts, "target-endpoint-folder-id", ""); value != "" {
		options["targetEndpointFolderId"] = toInt(value, 0)
	}
	if includeSchema {
		options["schemaOverwriteBehavior"] = optString(opts, "schema-overwrite-behavior", "OVERWRITE_EXISTING")
		if value := optString(opts, "target-schema-folder-id", ""); value != "" {
			options["targetSchemaFolderId"] = toInt(value, 0)
		}
		options["prependBasePath"] = optBool(opts, "prepend-base-path")
	}
	if value := optString(opts, "endpoint-case-overwrite-behavior", ""); value != "" {
		options["endpointCaseOverwriteBehavior"] = value
	}
	return options
}

func (a *App) cmdImportOpenAPI(args []string) error {
	valueFlags := map[string]bool{"file": true, "url": true, "basic-auth-username": true, "basic-auth-password": true, "target-endpoint-folder-id": true, "endpoint-overwrite-behavior": true, "target-schema-folder-id": true, "schema-overwrite-behavior": true, "locale": true}
	opts, _, err := parseOptions(args, valueFlags, nil)
	if err != nil {
		return err
	}
	if err := a.requireConfig(); err != nil {
		return err
	}
	file := optString(opts, "file", "")
	sourceURL := optString(opts, "url", "")
	if (file == "" && sourceURL == "") || (file != "" && sourceURL != "") {
		return fail(2, "exactly one of --file or --url is required")
	}
	var input any
	if sourceURL != "" {
		input = map[string]any{"url": sourceURL}
		username := optString(opts, "basic-auth-username", "")
		password := optString(opts, "basic-auth-password", "")
		if username != "" || password != "" {
			if username == "" || password == "" {
				return fail(2, "--basic-auth-username and --basic-auth-password must be used together")
			}
			input.(map[string]any)["basicAuth"] = map[string]any{"username": username, "password": password}
		}
	} else {
		text, err := readText(file)
		if err != nil {
			return err
		}
		input = text
	}
	payload := map[string]any{"input": input, "options": importOptions(opts, true)}
	endpoint := "/projects/" + a.Config.ProjectID + "/import-openapi"
	locale := optString(opts, "locale", "zh-CN")
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printRequestPreview("POST", endpoint, payload, nil, locale)
		return nil
	}
	result, err := a.callAPI("POST", endpoint, payload, url.Values{"locale": []string{locale}})
	if err != nil {
		return err
	}
	if optBool(opts, "json") {
		printJSON(result)
	} else {
		fmt.Println("OpenAPI/Swagger 导入请求已完成")
		printJSON(result)
	}
	return nil
}

func (a *App) cmdImportPostman(args []string) error {
	valueFlags := map[string]bool{"file": true, "target-endpoint-folder-id": true, "endpoint-overwrite-behavior": true, "endpoint-case-overwrite-behavior": true, "locale": true}
	opts, _, err := parseOptions(args, valueFlags, nil)
	if err != nil {
		return err
	}
	if err := a.requireConfig(); err != nil {
		return err
	}
	file := optString(opts, "file", "")
	if file == "" {
		return fail(2, "--file is required")
	}
	text, err := readText(file)
	if err != nil {
		return err
	}
	payload := map[string]any{"input": text, "options": importOptions(opts, false)}
	endpoint := "/projects/" + a.Config.ProjectID + "/import-postman-collection"
	locale := optString(opts, "locale", "zh-CN")
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printRequestPreview("POST", endpoint, payload, nil, locale)
		return nil
	}
	result, err := a.callAPI("POST", endpoint, payload, url.Values{"locale": []string{locale}})
	if err != nil {
		return err
	}
	if optBool(opts, "json") {
		printJSON(result)
	} else {
		fmt.Println("Postman Collection 导入请求已完成")
		printJSON(result)
	}
	return nil
}

func printRequestPreview(method string, endpoint string, payload any, params map[string]any, locale string) {
	preview := map[string]any{"method": strings.ToUpper(method), "endpoint": endpoint}
	if locale != "" {
		preview["locale"] = locale
	}
	if params != nil && len(params) > 0 {
		preview["params"] = params
	}
	if payload != nil {
		preview["payload"] = payload
	}
	printJSON(preview)
}

func (a *App) cmdVersions(args []string) error {
	opts, _, err := parseOptions(args, map[string]bool{"output": true}, map[string]string{"o": "output"})
	if err != nil {
		return err
	}
	result, err := a.callAPI("GET", "/versions", nil, nil)
	if err != nil {
		return err
	}
	text := jsonPretty(result)
	if file := optString(opts, "output", ""); file != "" {
		if err := writeOutput(file, text); err != nil {
			return err
		}
		fmt.Printf("已写入 %s\n", file)
		return nil
	}
	fmt.Println(text)
	return nil
}

func (a *App) cmdRequest(args []string) error {
	valueFlags := map[string]bool{"query": true, "data": true, "data-file": true, "output": true, "locale": true}
	opts, pos, err := parseOptions(args, valueFlags, map[string]string{"o": "output"})
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return fail(2, "request requires METHOD and endpoint")
	}
	method := strings.ToUpper(pos[0])
	endpoint := pos[1]
	if !strings.HasPrefix(endpoint, "/") {
		return fail(2, "endpoint must start with '/', for example /versions")
	}
	if strings.HasPrefix(endpoint, "/v1/") {
		return fail(2, "endpoint must be relative to /v1, for example /versions")
	}
	dataRaw := optString(opts, "data", "")
	dataFile := optString(opts, "data-file", "")
	if dataRaw != "" && dataFile != "" {
		return fail(2, "--data and --data-file cannot be used together")
	}
	var payload any
	if dataRaw != "" {
		obj, err := parseJSONObject(dataRaw, "--data")
		if err != nil {
			return err
		}
		payload = obj
	} else if dataFile != "" {
		obj, err := readJSONObject(dataFile, "--data-file")
		if err != nil {
			return err
		}
		payload = obj
	}
	queryValues := url.Values{}
	queryPreview := map[string]any{}
	for _, item := range opts["query"] {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			return fail(2, "query parameter must use key=value form: %s", item)
		}
		parsed := parseValue(value)
		queryValues.Add(key, toString(parsed))
		queryPreview[key] = parsed
	}
	locale := optString(opts, "locale", "zh-CN")
	if optBool(opts, "dry-run") || optBool(opts, "print-payload") {
		printRequestPreview(method, endpoint, payload, queryPreview, locale)
		return nil
	}
	if locale != "" {
		queryValues.Set("locale", locale)
	}
	result, err := a.callAPI(method, endpoint, payload, queryValues)
	if err != nil {
		return err
	}
	text := jsonPretty(result)
	if file := optString(opts, "output", ""); file != "" {
		if err := writeOutput(file, text); err != nil {
			return err
		}
		fmt.Printf("已写入 %s\n", file)
		return nil
	}
	fmt.Println(text)
	return nil
}

func (a *App) checkConfig() (string, error) {
	lines := []string{"Apifox 配置检查", strings.Repeat("=", 40)}
	if a.Config.Token != "" {
		token := "***"
		if len(a.Config.Token) > 12 {
			token = a.Config.Token[:8] + "..." + a.Config.Token[len(a.Config.Token)-4:]
		}
		lines = append(lines, "APIFOX_TOKEN: "+token)
	} else {
		lines = append(lines, "APIFOX_TOKEN: 未设置")
	}
	if a.Config.ProjectID != "" {
		lines = append(lines, "APIFOX_PROJECT_ID: "+a.Config.ProjectID)
	} else {
		lines = append(lines, "APIFOX_PROJECT_ID: 未设置")
	}
	lines = append(lines, "API 版本: "+apiVersion, "使用公开 API: "+a.Config.BaseURL+"/v1")
	if a.Config.Token == "" || a.Config.ProjectID == "" {
		lines = append(lines, "", "请设置 APIFOX_TOKEN 和 APIFOX_PROJECT_ID")
		return strings.Join(lines, "\n"), nil
	}
	openapi, err := a.exportOpenAPIMap(false, false)
	if err != nil {
		lines = append(lines, "", "API 连接失败: "+err.Error())
		return strings.Join(lines, "\n"), nil
	}
	info, _ := toMap(openapi["info"])
	paths, _ := toMap(openapi["paths"])
	components, _ := toMap(openapi["components"])
	schemas, _ := toMap(components["schemas"])
	lines = append(lines, "", "API 连接成功", "项目名称: "+defaultString(toString(info["title"]), "未知项目"), fmt.Sprintf("接口数量: %d 个", len(paths)), fmt.Sprintf("数据模型: %d 个", len(schemas)))
	return strings.Join(lines, "\n"), nil
}

func (a *App) listAPIEndpoints(keyword string, limit int) (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	type apiInfo struct {
		Method string
		Path   string
		Name   string
		Tags   string
	}
	var apis []apiInfo
	for path, rawMethods := range paths {
		methods, _ := toMap(rawMethods)
		for method, rawOp := range methods {
			if !httpMethods[strings.ToUpper(method)] {
				continue
			}
			op, _ := toMap(rawOp)
			name := defaultString(toString(op["summary"]), toString(op["operationId"]))
			if keyword != "" && !strings.Contains(strings.ToLower(name+" "+path), strings.ToLower(keyword)) {
				continue
			}
			var tags []string
			if values, ok := toSlice(op["tags"]); ok {
				for _, value := range values {
					tags = append(tags, toString(value))
				}
			}
			apis = append(apis, apiInfo{Method: strings.ToUpper(method), Path: path, Name: defaultString(name, "未命名"), Tags: strings.Join(tags, ", ")})
		}
	}
	sort.Slice(apis, func(i, j int) bool {
		if apis[i].Path == apis[j].Path {
			return apis[i].Method < apis[j].Method
		}
		return apis[i].Path < apis[j].Path
	})
	if len(apis) == 0 {
		return "当前项目中没有接口", nil
	}
	if limit <= 0 {
		limit = 50
	}
	lines := []string{fmt.Sprintf("接口列表 (共 %d 个)", len(apis)), strings.Repeat("=", 70)}
	for index, api := range apis {
		if index >= limit {
			break
		}
		line := fmt.Sprintf("[%-6s] %-40s | %s", api.Method, api.Path, api.Name)
		if api.Tags != "" {
			line += " [" + api.Tags + "]"
		}
		lines = append(lines, line)
	}
	if len(apis) > limit {
		lines = append(lines, fmt.Sprintf("\n... 还有 %d 个接口未显示", len(apis)-limit))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) getEndpointDetail(path string, method string) (string, error) {
	if path == "" || method == "" {
		return "", fail(2, "path and method are required")
	}
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	pathItem, ok := toMap(paths[path])
	if !ok {
		return "未找到路径为 " + path + " 的接口", nil
	}
	op, ok := toMap(pathItem[strings.ToLower(method)])
	if !ok {
		return "未找到 " + strings.ToUpper(method) + " " + path + " 接口", nil
	}
	params, _ := toSlice(op["parameters"])
	responses, _ := toMap(op["responses"])
	var tags []string
	if values, ok := toSlice(op["tags"]); ok {
		for _, value := range values {
			tags = append(tags, toString(value))
		}
	}
	lines := []string{
		"接口详情: " + defaultString(toString(op["summary"]), "未命名"),
		strings.Repeat("=", 50),
		"路径: " + strings.ToUpper(method) + " " + path,
		"说明: " + defaultString(toString(op["description"]), "无"),
		"标签: " + defaultString(strings.Join(tags, ", "), "无"),
		"",
		fmt.Sprintf("参数 (%d 个):", len(params)),
	}
	for _, raw := range params {
		param, _ := toMap(raw)
		schema, _ := toMap(param["schema"])
		lines = append(lines, fmt.Sprintf("  - [%s] %s: %s", toString(param["in"]), toString(param["name"]), defaultString(toString(schema["type"]), "any")))
	}
	if len(params) == 0 {
		lines = append(lines, "  无")
	}
	lines = append(lines, "", fmt.Sprintf("响应 (%d 个):", len(responses)))
	var codes []string
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		resp, _ := toMap(responses[code])
		lines = append(lines, fmt.Sprintf("  - %s: %s", code, toString(resp["description"])))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) listSchemas(keyword string, limit int) (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	components, _ := toMap(openapi["components"])
	schemas, _ := toMap(components["schemas"])
	if len(schemas) == 0 {
		return "当前项目中没有数据模型", nil
	}
	var names []string
	for name := range schemas {
		if keyword == "" || strings.Contains(strings.ToLower(name), strings.ToLower(keyword)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	lines := []string{fmt.Sprintf("数据模型列表 (共 %d 个)", len(names)), strings.Repeat("=", 50)}
	if limit <= 0 {
		limit = 50
	}
	for index, name := range names {
		if index >= limit {
			break
		}
		schema, _ := toMap(schemas[name])
		properties, _ := toMap(schema["properties"])
		lines = append(lines, fmt.Sprintf("- [%-8s] %s (%d 个属性)", defaultString(toString(schema["type"]), "object"), name, len(properties)))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) getSchemaDetail(name string) (string, error) {
	if name == "" {
		return "", fail(2, "name is required")
	}
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	components, _ := toMap(openapi["components"])
	schemas, _ := toMap(components["schemas"])
	schema, ok := toMap(schemas[name])
	if !ok {
		return "未找到名为 " + name + " 的数据模型", nil
	}
	properties, _ := toMap(schema["properties"])
	required := map[string]bool{}
	if values, ok := toSlice(schema["required"]); ok {
		for _, value := range values {
			required[toString(value)] = true
		}
	}
	lines := []string{"数据模型详情: " + name, strings.Repeat("=", 50), "说明: " + defaultString(toString(schema["description"]), "无"), "类型: " + defaultString(toString(schema["type"]), "unknown"), ""}
	if len(properties) > 0 {
		lines = append(lines, fmt.Sprintf("属性列表 (%d 个):", len(properties)))
		var names []string
		for propName := range properties {
			names = append(names, propName)
		}
		sort.Strings(names)
		for _, propName := range names {
			prop, _ := toMap(properties[propName])
			mark := " "
			if required[propName] {
				mark = "*"
			}
			lines = append(lines, fmt.Sprintf("%s %s: %s", mark, propName, defaultString(toString(prop["type"]), "any")))
		}
	}
	lines = append(lines, "\n* 表示必填字段")
	return strings.Join(lines, "\n"), nil
}

func (a *App) listTags() (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	counts := map[string]int{}
	for _, rawMethods := range paths {
		methods, _ := toMap(rawMethods)
		for method, rawOp := range methods {
			if !httpMethods[strings.ToUpper(method)] {
				continue
			}
			op, _ := toMap(rawOp)
			if values, ok := toSlice(op["tags"]); ok {
				for _, value := range values {
					tag := defaultString(toString(value), "未分类")
					counts[tag]++
				}
			} else {
				counts["未分类"]++
			}
		}
	}
	var names []string
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] == counts[names[j]] {
			return names[i] < names[j]
		}
		return counts[names[i]] > counts[names[j]]
	})
	lines := []string{fmt.Sprintf("标签列表 (共 %d 个)", len(names)), strings.Repeat("=", 50)}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("- %s: %d 个接口", name, counts[name]))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) getAPIsByTag(tag string) (string, error) {
	if tag == "" {
		return "", fail(2, "tag is required")
	}
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	var lines []string
	for path, rawMethods := range paths {
		methods, _ := toMap(rawMethods)
		for method, rawOp := range methods {
			op, _ := toMap(rawOp)
			if values, ok := toSlice(op["tags"]); ok {
				for _, value := range values {
					if toString(value) == tag {
						lines = append(lines, fmt.Sprintf("[%-6s] %-40s | %s", strings.ToUpper(method), path, defaultString(toString(op["summary"]), "未命名")))
					}
				}
			}
		}
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return "标签 '" + tag + "' 下没有接口", nil
	}
	return strings.Join(append([]string{"标签: " + tag, fmt.Sprintf("接口列表 (共 %d 个)", len(lines)), strings.Repeat("=", 70)}, lines...), "\n"), nil
}

func (a *App) listFolders() (string, error) {
	return a.listTags()
}

func (a *App) deleteEndpoint(path string, method string, confirm bool) (string, error) {
	if path == "" || method == "" {
		return "", fail(2, "path and method are required")
	}
	method = strings.ToUpper(method)
	if !httpMethods[method] {
		return "", fail(2, "method must be one of: %s", strings.Join(sortedKeys(httpMethods), ", "))
	}
	if !confirm {
		return fmt.Sprintf("删除操作不可撤销。\n\n确认要删除接口时再执行:\napifox-mcp api delete --method %s --path %s --confirm", method, path), nil
	}
	return fmt.Sprintf("Apifox 公开 API 暂不支持直接删除接口。\n\n请在 Apifox 客户端中手动删除: %s %s", method, path), nil
}

func (a *App) deleteSchema(name string, confirm bool) (string, error) {
	if name == "" {
		return "", fail(2, "name is required")
	}
	if !confirm {
		return fmt.Sprintf("删除操作不可撤销。\n\n确认要删除数据模型时再执行:\napifox-mcp schema delete --name %s --confirm", name), nil
	}
	return fmt.Sprintf("Apifox 公开 API 暂不支持直接删除数据模型。\n\n请在 Apifox 客户端中手动删除: %s", name), nil
}

func (a *App) createFolder(name string, description string) (string, error) {
	if name == "" {
		return "", fail(2, "name is required")
	}
	_ = description
	return fmt.Sprintf("Apifox 的目录可通过接口标签自动形成。\n\n创建或更新接口时设置 tags 包含 %q，或执行:\napifox-mcp tag add --method GET --path /existing-api --tag %s", name, name), nil
}

func (a *App) deleteFolder(name string, confirm bool) (string, error) {
	if name == "" {
		return "", fail(2, "name is required")
	}
	if !confirm {
		return fmt.Sprintf("删除操作不可撤销。\n\n确认要删除目录时再执行:\napifox-mcp folder delete --name %s --confirm", name), nil
	}
	return fmt.Sprintf("Apifox 公开 API 暂不支持直接删除目录。\n\n请在 Apifox 客户端中手动删除目录: %s", name), nil
}

func (a *App) setEndpointTags(path string, method string, tags []string) (string, error) {
	if path == "" || method == "" {
		return "", fail(2, "path and method are required")
	}
	method = strings.ToUpper(method)
	if !httpMethods[method] {
		return "", fail(2, "method must be one of: %s", strings.Join(sortedKeys(httpMethods), ", "))
	}
	if len(tags) == 0 {
		return "", fail(2, "at least one --tag is required")
	}
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	pathItem, ok := toMap(paths[path])
	if !ok {
		return "", fail(1, "未找到路径为 %s 的接口", path)
	}
	operation, ok := toMap(pathItem[strings.ToLower(method)])
	if !ok {
		return "", fail(1, "未找到 %s %s 接口", method, path)
	}
	tagValues := make([]any, 0, len(tags))
	for _, tag := range tags {
		tagValues = append(tagValues, tag)
	}
	operation["tags"] = tagValues
	components := map[string]any{}
	if rawComponents, ok := toMap(openapi["components"]); ok {
		components = rawComponents
	}
	importSpec := map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": defaultString(toString(operation["summary"]), "API"), "version": "1.0.0"},
		"paths":   map[string]any{path: map[string]any{strings.ToLower(method): operation}},
	}
	if len(components) > 0 {
		importSpec["components"] = components
	}
	result, err := a.importOpenAPI(importSpec, "OVERWRITE_EXISTING", "OVERWRITE_EXISTING", 0)
	if err != nil {
		return "", err
	}
	counters := importCounters(result)
	return fmt.Sprintf("标签更新成功\n\n接口: %s %s\n标签: %s\n更新: %d", method, path, strings.Join(tags, ", "), counters["endpointUpdated"]), nil
}

func responseCodes(op map[string]any) map[string]bool {
	out := map[string]bool{}
	responses, _ := toMap(op["responses"])
	for code := range responses {
		out[code] = true
	}
	return out
}

func responseProblems(op map[string]any, method string) []string {
	codes := responseCodes(op)
	var problems []string
	has2xx := false
	for code := range codes {
		if successCodeRE.MatchString(code) {
			has2xx = true
			break
		}
	}
	if !has2xx {
		problems = append(problems, "缺少 2xx 成功响应")
	}
	for _, code := range []string{"400", "401", "403", "404", "500", "502", "503"} {
		if !codes[code] {
			problems = append(problems, "缺少 "+code+" 响应")
		}
	}
	if method == "post" || method == "put" || method == "patch" {
		for _, code := range []string{"409", "422"} {
			if !codes[code] {
				problems = append(problems, "缺少 "+code+" 响应")
			}
		}
	}
	return problems
}

func (a *App) checkAPIResponses(path string, method string) (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	pathItem, ok := toMap(paths[path])
	if !ok {
		return "未找到路径为 " + path + " 的接口", nil
	}
	op, ok := toMap(pathItem[strings.ToLower(method)])
	if !ok {
		return "未找到 " + strings.ToUpper(method) + " " + path + " 接口", nil
	}
	problems := responseProblems(op, strings.ToLower(method))
	if len(problems) == 0 {
		return "响应定义完整: " + strings.ToUpper(method) + " " + path, nil
	}
	return "响应定义不完整: " + strings.ToUpper(method) + " " + path + "\n- " + strings.Join(problems, "\n- "), nil
}

func (a *App) auditAllAPIResponses(tag string, showComplete bool) (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	var problemLines, completeLines []string
	for path, rawMethods := range paths {
		methods, _ := toMap(rawMethods)
		for method, rawOp := range methods {
			if !httpMethods[strings.ToUpper(method)] {
				continue
			}
			op, _ := toMap(rawOp)
			if tag != "" {
				found := false
				if values, ok := toSlice(op["tags"]); ok {
					for _, value := range values {
						if toString(value) == tag {
							found = true
							break
						}
					}
				}
				if !found {
					continue
				}
			}
			problems := responseProblems(op, method)
			line := fmt.Sprintf("%s %s | %s", strings.ToUpper(method), path, defaultString(toString(op["summary"]), "未命名"))
			if len(problems) > 0 {
				problemLines = append(problemLines, line+": "+strings.Join(problems, ", "))
			} else if showComplete {
				completeLines = append(completeLines, line)
			}
		}
	}
	sort.Strings(problemLines)
	sort.Strings(completeLines)
	lines := []string{fmt.Sprintf("响应审计: %d 个问题接口", len(problemLines)), strings.Repeat("=", 70)}
	lines = append(lines, problemLines...)
	if showComplete {
		lines = append(lines, "", fmt.Sprintf("完整接口: %d 个", len(completeLines)))
		lines = append(lines, completeLines...)
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) checkPathNaming(style string) (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	var bad []string
	for path := range paths {
		for _, segment := range strings.Split(path, "/") {
			if segment == "" || strings.HasPrefix(segment, "{") {
				continue
			}
			ok := true
			switch style {
			case "kebab-case":
				ok = kebabSegmentRE.MatchString(segment)
			case "snake_case":
				ok = snakeSegmentRE.MatchString(segment)
			case "camelCase":
				ok = camelSegmentRE.MatchString(segment)
			default:
				return "", fail(2, "unsupported style: %s", style)
			}
			if !ok {
				bad = append(bad, path)
				break
			}
		}
	}
	sort.Strings(bad)
	if len(bad) == 0 {
		return "所有路径符合 " + style + " 命名规范", nil
	}
	return fmt.Sprintf("发现 %d 个路径不符合 %s:\n- %s", len(bad), style, strings.Join(bad, "\n- ")), nil
}

func (a *App) checkResponseConsistency() (string, error) {
	openapi, err := a.exportOpenAPIMap(false, true)
	if err != nil {
		return "", err
	}
	paths, _ := toMap(openapi["paths"])
	var missing []string
	for path, rawMethods := range paths {
		methods, _ := toMap(rawMethods)
		for method, rawOp := range methods {
			if !httpMethods[strings.ToUpper(method)] {
				continue
			}
			op, _ := toMap(rawOp)
			problems := responseProblems(op, method)
			if len(problems) > 0 {
				missing = append(missing, fmt.Sprintf("%s %s: %s", strings.ToUpper(method), path, strings.Join(problems, ", ")))
			}
		}
	}
	sort.Strings(missing)
	if len(missing) == 0 {
		return "响应格式检查通过", nil
	}
	return fmt.Sprintf("响应格式存在不一致或缺失 (%d 个):\n- %s", len(missing), strings.Join(missing, "\n- ")), nil
}
