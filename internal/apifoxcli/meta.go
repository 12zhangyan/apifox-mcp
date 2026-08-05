package apifoxcli

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type endpointMetaSpec struct {
	Method   string
	Path     string
	Tags     []string
	HasTags  bool
	Folder   string
	FolderID *int
}

type httpAPIRecord struct {
	ID       int
	Name     string
	Method   string
	Path     string
	FolderID int
	Tags     []string
}

type folderRecord struct {
	ID          int
	Name        string
	Description string
	ParentID    int
	Path        string
}

func endpointMetaSpecFromFlags(method string, path string, tags []string, opts map[string][]string) (endpointMetaSpec, error) {
	if len(tags) == 0 {
		return endpointMetaSpec{}, fail(2, "at least one --tag is required")
	}
	spec := endpointMetaSpec{
		Method:  strings.ToUpper(strings.TrimSpace(method)),
		Path:    strings.TrimSpace(path),
		Tags:    tags,
		HasTags: true,
		Folder:  strings.Trim(strings.TrimSpace(optString(opts, "folder", "")), "/"),
	}
	if raw := optString(opts, "folder-id", ""); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return endpointMetaSpec{}, fail(2, "--folder-id must be a non-negative integer")
		}
		spec.FolderID = &value
	}
	if spec.Folder != "" && spec.FolderID != nil {
		return endpointMetaSpec{}, fail(2, "--folder and --folder-id cannot be used together")
	}
	if optBool(opts, "sync-folder") {
		if spec.Folder != "" || spec.FolderID != nil {
			return endpointMetaSpec{}, fail(2, "--sync-folder cannot be combined with --folder or --folder-id")
		}
		spec.Folder = strings.Trim(tags[0], "/")
	}
	if err := validateEndpointMetaSpec(spec, true); err != nil {
		return endpointMetaSpec{}, err
	}
	return spec, nil
}

func validateEndpointMetaSpec(spec endpointMetaSpec, requireTags bool) error {
	if spec.Method == "" || spec.Path == "" {
		return fail(2, "method and path are required")
	}
	if !httpMethods[spec.Method] {
		return fail(2, "method must be one of: %s", strings.Join(sortedKeys(httpMethods), ", "))
	}
	if !strings.HasPrefix(spec.Path, "/") {
		return fail(2, "path must start with '/': %s", spec.Path)
	}
	if requireTags && !spec.HasTags {
		return fail(2, "tags are required for every tag batch operation")
	}
	if !spec.HasTags && spec.Folder == "" && spec.FolderID == nil {
		return fail(2, "each operation must provide tags, folder, or folder_id")
	}
	return nil
}

func readEndpointMetaSpecs(path string, requireTags bool) ([]endpointMetaSpec, error) {
	if path == "" {
		return nil, fail(2, "--file is required")
	}
	data, err := readJSON(path)
	if err != nil {
		return nil, err
	}
	items, ok := toSlice(data)
	if !ok {
		obj, objectOK := toMap(data)
		if !objectOK {
			return nil, fail(2, "batch input must be a JSON array or object")
		}
		items, ok = toSlice(obj["operations"])
		if !ok {
			return nil, fail(2, "batch input object must contain an operations array")
		}
	}
	if len(items) == 0 {
		return nil, fail(2, "operations must not be empty")
	}
	specs := make([]endpointMetaSpec, 0, len(items))
	for index, item := range items {
		obj, ok := toMap(item)
		if !ok {
			return nil, fail(2, "operations[%d] must be an object", index)
		}
		spec := endpointMetaSpec{
			Method: strings.ToUpper(strings.TrimSpace(toString(obj["method"]))),
			Path:   strings.TrimSpace(toString(obj["path"])),
			Folder: strings.Trim(strings.TrimSpace(toString(obj["folder"])), "/"),
		}
		if rawTags, exists := obj["tags"]; exists {
			spec.HasTags = true
			tagItems, ok := toSlice(rawTags)
			if !ok {
				return nil, fail(2, "operations[%d].tags must be an array", index)
			}
			for _, rawTag := range tagItems {
				tag := strings.TrimSpace(toString(rawTag))
				if tag == "" {
					return nil, fail(2, "operations[%d].tags must not contain empty values", index)
				}
				spec.Tags = append(spec.Tags, tag)
			}
		}
		if rawFolderID, exists := obj["folder_id"]; exists {
			folderID := toInt(rawFolderID, -1)
			if folderID < 0 {
				return nil, fail(2, "operations[%d].folder_id must be a non-negative integer", index)
			}
			spec.FolderID = &folderID
		}
		if toBool(obj["sync_folder"], false) {
			if !spec.HasTags || len(spec.Tags) == 0 {
				return nil, fail(2, "operations[%d].sync_folder requires at least one tag", index)
			}
			if spec.Folder != "" || spec.FolderID != nil {
				return nil, fail(2, "operations[%d].sync_folder conflicts with folder/folder_id", index)
			}
			spec.Folder = strings.Trim(spec.Tags[0], "/")
		}
		if spec.Folder != "" && spec.FolderID != nil {
			return nil, fail(2, "operations[%d] cannot contain both folder and folder_id", index)
		}
		if err := validateEndpointMetaSpec(spec, requireTags); err != nil {
			return nil, fail(2, "operations[%d]: %s", index, err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func unwrapAPIData(result any) any {
	obj, ok := toMap(result)
	if !ok {
		return result
	}
	if data, exists := obj["data"]; exists {
		return data
	}
	return result
}

func objectItems(value any, keys ...string) []map[string]any {
	if items, ok := toSlice(value); ok {
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if obj, ok := toMap(item); ok {
				out = append(out, obj)
			}
		}
		return out
	}
	obj, ok := toMap(value)
	if !ok {
		return nil
	}
	for _, key := range keys {
		if items := objectItems(obj[key], keys...); items != nil {
			return items
		}
	}
	return nil
}

func stringItems(value any) []string {
	items, _ := toSlice(value)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(toString(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func (a *App) listHTTPAPIRecords() ([]httpAPIRecord, error) {
	if err := a.requireConfig(); err != nil {
		return nil, err
	}
	const pageSize = 100
	records := make([]httpAPIRecord, 0)
	seen := map[int]bool{}
	for page := 1; ; {
		params := url.Values{
			"page":    []string{strconv.Itoa(page)},
			"perPage": []string{strconv.Itoa(pageSize)},
		}
		result, err := a.callDesignAPI(http.MethodGet, "/projects/"+a.Config.ProjectID+"/http-apis", nil, params)
		if err != nil {
			return nil, err
		}
		items := objectItems(unwrapAPIData(result), "items", "list", "httpApis", "data")
		for _, item := range items {
			id := toInt(item["id"], 0)
			if id <= 0 {
				id = toInt(item["apiDetailId"], 0)
			}
			method := strings.ToUpper(toString(item["method"]))
			path := toString(item["path"])
			if id <= 0 || method == "" || path == "" || seen[id] {
				continue
			}
			seen[id] = true
			records = append(records, httpAPIRecord{
				ID:       id,
				Name:     toString(item["name"]),
				Method:   method,
				Path:     path,
				FolderID: toInt(item["folderId"], 0),
				Tags:     stringItems(item["tags"]),
			})
		}

		response, _ := toMap(result)
		meta, hasMeta := toMap(response["meta"])
		nextPage := toInt(meta["nextPage"], 0)
		if nextPage > page {
			page = nextPage
			continue
		}
		totalPages := toInt(meta["totalPages"], 0)
		if totalPages > page {
			page++
			continue
		}
		if !hasMeta || nextPage <= page {
			break
		}
	}
	return records, nil
}

func findHTTPAPI(records []httpAPIRecord, method string, path string) (httpAPIRecord, error) {
	for _, record := range records {
		if record.Method == method && record.Path == path {
			return record, nil
		}
	}
	return httpAPIRecord{}, fail(1, "endpoint not found: %s %s", method, path)
}

func flattenFolderItems(value any, inferredParent int, out *[]folderRecord) {
	for _, item := range objectItems(value, "items", "list", "folders", "data") {
		id := toInt(item["id"], 0)
		name := strings.TrimSpace(toString(item["name"]))
		if id <= 0 || name == "" {
			continue
		}
		parentID := toInt(item["parentId"], inferredParent)
		*out = append(*out, folderRecord{ID: id, Name: name, Description: toString(item["description"]), ParentID: parentID})
		if children, ok := toSlice(item["children"]); ok {
			flattenFolderItems(children, id, out)
		}
	}
}

func folderPath(record folderRecord, byID map[int]folderRecord, visiting map[int]bool) string {
	if record.ParentID == 0 || visiting[record.ID] {
		return record.Name
	}
	parent, ok := byID[record.ParentID]
	if !ok {
		return record.Name
	}
	visiting[record.ID] = true
	path := folderPath(parent, byID, visiting) + "/" + record.Name
	delete(visiting, record.ID)
	return path
}

func (a *App) listFolderRecords() ([]folderRecord, error) {
	if err := a.requireConfig(); err != nil {
		return nil, err
	}
	result, err := a.callDesignAPI(http.MethodGet, "/projects/"+a.Config.ProjectID+"/api-folders", nil, nil)
	if err != nil {
		return nil, err
	}
	var records []folderRecord
	flattenFolderItems(unwrapAPIData(result), 0, &records)
	byID := make(map[int]folderRecord, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	for index := range records {
		records[index].Path = folderPath(records[index], byID, map[int]bool{})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

func findFolderByPath(records []folderRecord, path string) (folderRecord, error) {
	normalized := strings.Trim(strings.TrimSpace(path), "/")
	for _, record := range records {
		if record.Path == normalized {
			return record, nil
		}
	}
	return folderRecord{}, fail(1, "endpoint folder not found: %s", normalized)
}

func findFolderByID(records []folderRecord, id int) (folderRecord, error) {
	if id == 0 {
		return folderRecord{ID: 0, Name: "root", Path: ""}, nil
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return folderRecord{}, fail(1, "endpoint folder not found: %d", id)
}

func (a *App) applyEndpointMetaChanges(specs []endpointMetaSpec, dryRun bool) (commandResult, error) {
	endpoints, err := a.listHTTPAPIRecords()
	if err != nil {
		return commandResult{}, err
	}
	needsFolders := false
	for _, spec := range specs {
		needsFolders = needsFolders || spec.Folder != "" || spec.FolderID != nil
	}
	var folders []folderRecord
	if needsFolders {
		folders, err = a.listFolderRecords()
		if err != nil {
			return commandResult{}, err
		}
	}
	operations := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		endpoint, err := findHTTPAPI(endpoints, spec.Method, spec.Path)
		if err != nil {
			return commandResult{}, err
		}
		payload := map[string]any{}
		if spec.HasTags {
			payload["tags"] = spec.Tags
		}
		folderPath := ""
		if spec.Folder != "" {
			folder, err := findFolderByPath(folders, spec.Folder)
			if err != nil {
				return commandResult{}, err
			}
			payload["folderId"] = folder.ID
			folderPath = folder.Path
		} else if spec.FolderID != nil {
			folder, err := findFolderByID(folders, *spec.FolderID)
			if err != nil {
				return commandResult{}, err
			}
			payload["folderId"] = folder.ID
			folderPath = folder.Path
		}
		operations = append(operations, map[string]any{
			"endpoint_id": endpoint.ID,
			"method":      endpoint.Method,
			"path":        endpoint.Path,
			"folder":      folderPath,
			"payload":     payload,
		})
	}
	if !dryRun {
		for _, operation := range operations {
			endpointID := toInt(operation["endpoint_id"], 0)
			payload, _ := toMap(operation["payload"])
			endpoint := "/projects/" + a.Config.ProjectID + "/http-apis/" + strconv.Itoa(endpointID)
			if _, err := a.callDesignAPI(http.MethodPut, endpoint, payload, nil); err != nil {
				return commandResult{}, err
			}
		}
	}
	counters := map[string]any{"planned": len(operations), "updated": 0}
	if !dryRun {
		counters["updated"] = len(operations)
	}
	result := map[string]any{
		"kind":       "endpoint_meta",
		"dry_run":    dryRun,
		"operations": operations,
		"counters":   counters,
	}
	verb := "Updated"
	if dryRun {
		verb = "Would update"
	}
	return commandResult{Text: fmt.Sprintf("%s %d endpoint(s) through the lightweight API", verb, len(operations)), JSON: result}, nil
}

func folderJSON(record folderRecord) map[string]any {
	return map[string]any{
		"id":          record.ID,
		"name":        record.Name,
		"description": record.Description,
		"parent_id":   record.ParentID,
		"path":        record.Path,
	}
}

func (a *App) listRealFolders(rootID int) (commandResult, error) {
	records, err := a.listFolderRecords()
	if err != nil {
		return commandResult{}, err
	}
	rootPath := ""
	if rootID >= 0 {
		root, err := findFolderByID(records, rootID)
		if err != nil {
			return commandResult{}, err
		}
		rootPath = root.Path
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if rootID >= 0 && record.ID != rootID && !strings.HasPrefix(record.Path, rootPath+"/") {
			continue
		}
		items = append(items, folderJSON(record))
	}
	return commandResult{
		Text: fmt.Sprintf("Endpoint folders (%d)\n%s", len(items), jsonPretty(items)),
		JSON: map[string]any{"folders": items, "total": len(items)},
	}, nil
}

func (a *App) createEndpointFolder(name string, parentID int, description string, dryRun bool) (commandResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return commandResult{}, fail(2, "folder name is required")
	}
	if parentID < 0 {
		return commandResult{}, fail(2, "--parent-id must be a non-negative integer")
	}
	if parentID != 0 {
		folders, err := a.listFolderRecords()
		if err != nil {
			return commandResult{}, err
		}
		if _, err := findFolderByID(folders, parentID); err != nil {
			return commandResult{}, err
		}
	}
	payload := map[string]any{"name": name, "type": "http", "projectId": toInt(a.Config.ProjectID, 0), "parentId": parentID}
	if description != "" {
		payload["description"] = description
	}
	var response any
	if !dryRun {
		result, err := a.callDesignAPI(http.MethodPost, "/projects/"+a.Config.ProjectID+"/api-folders", payload, nil)
		if err != nil {
			return commandResult{}, err
		}
		response = unwrapAPIData(result)
	}
	jsonResult := map[string]any{"kind": "folder_create", "dry_run": dryRun, "payload": payload, "result": response}
	return commandResult{Text: fmt.Sprintf("Folder %q create dry_run=%t", name, dryRun), JSON: jsonResult}, nil
}

func (a *App) moveEndpointFolder(folderID int, parentID int, dryRun bool) (commandResult, error) {
	folders, err := a.listFolderRecords()
	if err != nil {
		return commandResult{}, err
	}
	folder, err := findFolderByID(folders, folderID)
	if err != nil {
		return commandResult{}, err
	}
	parent, err := findFolderByID(folders, parentID)
	if err != nil {
		return commandResult{}, err
	}
	if folderID == parentID || (parent.Path != "" && strings.HasPrefix(parent.Path, folder.Path+"/")) {
		return commandResult{}, fail(2, "cannot move a folder into itself or its descendant")
	}
	payload := map[string]any{"parentId": parentID}
	if !dryRun {
		endpoint := "/projects/" + a.Config.ProjectID + "/api-folders/" + strconv.Itoa(folderID)
		if _, err := a.callDesignAPI(http.MethodPut, endpoint, payload, nil); err != nil {
			return commandResult{}, err
		}
	}
	result := map[string]any{"kind": "folder_move", "dry_run": dryRun, "folder": folderJSON(folder), "payload": payload}
	return commandResult{Text: fmt.Sprintf("Folder %d move dry_run=%t", folderID, dryRun), JSON: result}, nil
}

func requiredIntOptionOrPos(opts map[string][]string, key string, pos []string, index int) (int, error) {
	raw := optOrPos(opts, key, pos, index)
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fail(2, "--%s must be a non-negative integer", key)
	}
	return value, nil
}

func readDeleteEmptyTarget(opts map[string][]string, pos []string) (*int, bool, error) {
	if file := optString(opts, "file", ""); file != "" {
		obj, err := readJSONObject(file, "folder delete-empty input")
		if err != nil {
			return nil, false, err
		}
		if toBool(obj["all"], false) {
			return nil, true, nil
		}
		folderID := toInt(obj["folder_id"], -1)
		if folderID <= 0 {
			return nil, false, fail(2, "folder_id must be a positive integer")
		}
		return &folderID, false, nil
	}
	if optBool(opts, "all") {
		return nil, true, nil
	}
	folderID, err := requiredIntOptionOrPos(opts, "folder-id", pos, 0)
	if err != nil || folderID == 0 {
		return nil, false, fail(2, "provide a positive --folder-id, --all, or --file")
	}
	return &folderID, false, nil
}

func folderDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}

func (a *App) deleteEmptyEndpointFolders(targetID *int, all bool, dryRun bool) (commandResult, error) {
	if !all && targetID == nil {
		return commandResult{}, fail(2, "folder delete-empty requires a target")
	}
	folders, err := a.listFolderRecords()
	if err != nil {
		return commandResult{}, err
	}
	endpoints, err := a.listHTTPAPIRecords()
	if err != nil {
		return commandResult{}, err
	}
	if targetID != nil {
		if _, err := findFolderByID(folders, *targetID); err != nil {
			return commandResult{}, err
		}
	}
	used := map[int]bool{}
	for _, endpoint := range endpoints {
		if endpoint.FolderID > 0 {
			used[endpoint.FolderID] = true
		}
	}
	byID := make(map[int]folderRecord, len(folders))
	for _, folder := range folders {
		byID[folder.ID] = folder
	}
	containsUsed := func(folder folderRecord) bool {
		for usedID := range used {
			current, ok := byID[usedID]
			for ok {
				if current.ID == folder.ID {
					return true
				}
				current, ok = byID[current.ParentID]
			}
		}
		return false
	}
	var selected []folderRecord
	if targetID != nil {
		root := byID[*targetID]
		if containsUsed(root) {
			return commandResult{}, fail(1, "folder subtree is not empty: %s", root.Path)
		}
		for _, folder := range folders {
			if folder.ID == root.ID || strings.HasPrefix(folder.Path, root.Path+"/") {
				selected = append(selected, folder)
			}
		}
	} else {
		for _, folder := range folders {
			if !containsUsed(folder) {
				selected = append(selected, folder)
			}
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		left, right := folderDepth(selected[i].Path), folderDepth(selected[j].Path)
		if left == right {
			return selected[i].Path < selected[j].Path
		}
		return left > right
	})
	operations := make([]map[string]any, 0, len(selected))
	for _, folder := range selected {
		operations = append(operations, folderJSON(folder))
		if !dryRun {
			endpoint := "/projects/" + a.Config.ProjectID + "/api-folders/" + strconv.Itoa(folder.ID)
			if _, err := a.callDesignAPI(http.MethodDelete, endpoint, nil, url.Values{}); err != nil {
				return commandResult{}, err
			}
		}
	}
	counters := map[string]any{"planned": len(selected), "deleted": 0}
	if !dryRun {
		counters["deleted"] = len(selected)
	}
	result := map[string]any{"kind": "folder_delete_empty", "dry_run": dryRun, "folders": operations, "counters": counters}
	return commandResult{Text: fmt.Sprintf("Empty folders selected: %d (dry_run=%t)", len(selected), dryRun), JSON: result}, nil
}
