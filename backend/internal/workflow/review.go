package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var severityRank = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

// Finding is the controlled finding contract produced by a validated model response.
type Finding struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Comment  string `json:"comment"`
	Severity string `json:"severity"`
}

type ValidationFailure struct {
	Errors []string `json:"errors"`
}

type FileGroup struct {
	Extension  string           `json:"extension,omitempty"`
	Files      []map[string]any `json:"files"`
	Characters int              `json:"characters"`
}

type Review struct {
	Findings []Finding `json:"findings"`
}

// FormattedReview is a destination-neutral, deterministic review payload.
type FormattedReview struct {
	Summary      ReviewSummary       `json:"summary"`
	Observations []ReviewObservation `json:"observations"`
	Findings     []Finding           `json:"findings"`
}

type ReviewSummary struct {
	Total    int    `json:"total"`
	Low      int    `json:"low"`
	Medium   int    `json:"medium"`
	High     int    `json:"high"`
	Critical int    `json:"critical"`
	Event    string `json:"event"`
	Status   string `json:"status"`
}

// ReviewObservation is the destination-neutral representation of one inline review comment.
type ReviewObservation struct {
	Path        string `json:"path"`
	Body        string `json:"body"`
	NewPosition int    `json:"new_position"`
}

func filterFiles(values []any, config map[string]any) ([]map[string]any, error) {
	include, err := configuredExtensions(config, "include_extensions")
	if err != nil {
		return nil, err
	}
	exclude, err := configuredExtensions(config, "exclude_extensions")
	if err != nil {
		return nil, err
	}
	ignoreGenerated, err := configuredBool(config, "ignore_generated", false)
	if err != nil {
		return nil, err
	}
	files, err := workflowFiles(values)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(files))
	for _, file := range files {
		filename := file["filename"].(string)
		extension := normalizedExtension(filename)
		if len(include) > 0 && !include[extension] {
			continue
		}
		if exclude[extension] || (ignoreGenerated && generatedFile(file)) {
			continue
		}
		result = append(result, file)
	}
	sortFiles(result)
	return result, nil
}

func groupFiles(values []any, config map[string]any) ([]FileGroup, error) {
	maxFiles, ok := integer(config["max_files"])
	if !ok || maxFiles < 1 {
		return nil, fmt.Errorf("group card requires positive config.max_files")
	}
	maxCharacters, ok := integer(config["max_characters"])
	if !ok || maxCharacters < 1 {
		return nil, fmt.Errorf("group card requires positive config.max_characters")
	}
	byExtension, err := configuredBool(config, "group_by_extension", false)
	if err != nil {
		return nil, err
	}
	files, err := workflowFiles(values)
	if err != nil {
		return nil, err
	}
	sortFiles(files)
	buckets := map[string][]map[string]any{"": files}
	keys := []string{""}
	if byExtension {
		buckets = map[string][]map[string]any{}
		keys = nil
		for _, file := range files {
			extension := normalizedExtension(file["filename"].(string))
			if _, exists := buckets[extension]; !exists {
				keys = append(keys, extension)
			}
			buckets[extension] = append(buckets[extension], file)
		}
		sort.Strings(keys)
	}
	groups := make([]FileGroup, 0)
	for _, extension := range keys {
		var current FileGroup
		if byExtension {
			current.Extension = extension
		}
		for _, file := range buckets[extension] {
			characters := fileCharacters(file)
			if len(current.Files) > 0 && (len(current.Files) >= maxFiles || current.Characters+characters > maxCharacters) {
				groups = append(groups, current)
				current = FileGroup{Extension: current.Extension}
			}
			current.Files = append(current.Files, file)
			current.Characters += characters
		}
		if len(current.Files) > 0 {
			groups = append(groups, current)
		}
	}
	return groups, nil
}

func validateResponse(responseValues, fileValues []any, config map[string]any) (any, string) {
	response, ok := firstValue(responseValues).(string)
	if !ok {
		return ValidationFailure{Errors: []string{"model response must be a JSON finding list"}}, "invalid"
	}
	findings, err := parseFindings(response)
	if err != nil {
		return ValidationFailure{Errors: []string{err.Error()}}, "invalid"
	}
	validatePaths, err := configuredBool(config, "validate_paths", false)
	if err != nil {
		return ValidationFailure{Errors: []string{err.Error()}}, "invalid"
	}
	var filesByPath map[string]map[string]any
	if validatePaths {
		if len(fileValues) == 0 {
			return ValidationFailure{Errors: []string{"validate card requires fetched files when config.validate_paths is true"}}, "invalid"
		}
		files, fileErr := workflowFiles(fileValues)
		if fileErr != nil {
			return ValidationFailure{Errors: []string{"validate card requires fetched files when config.validate_paths is true"}}, "invalid"
		}
		filesByPath = make(map[string]map[string]any, len(files))
		for _, file := range files {
			filesByPath[file["filename"].(string)] = file
		}
	}
	errors := make([]string, 0)
	for index, finding := range findings {
		if strings.TrimSpace(finding.Path) == "" {
			errors = append(errors, fmt.Sprintf("finding %d path is required", index))
		}
		if finding.Line < 1 {
			errors = append(errors, fmt.Sprintf("finding %d line must be positive", index))
		}
		if strings.TrimSpace(finding.Comment) == "" {
			errors = append(errors, fmt.Sprintf("finding %d comment is required", index))
		}
		if _, allowed := severityRank[finding.Severity]; !allowed {
			errors = append(errors, fmt.Sprintf("finding %d severity %q is not allowed", index, finding.Severity))
		}
		if validatePaths && filesByPath[finding.Path] == nil {
			errors = append(errors, fmt.Sprintf("finding %d path %q is not in fetched files", index, finding.Path))
		} else if validatePaths && !lineExistsInPatch(filesByPath[finding.Path], finding.Line) {
			errors = append(errors, fmt.Sprintf("finding %d line %d is not present in the changed lines of %q", index, finding.Line, finding.Path))
		}
	}
	if len(errors) > 0 {
		return ValidationFailure{Errors: errors}, "invalid"
	}
	return findings, "valid"
}

var unifiedDiffHunk = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// lineExistsInPatch verifies the source line number against the new side of a
// unified diff. Gitea's new_position is this new-file line number, not an
// offset in the patch; accepting merely a positive number caused comments to
// be published on unrelated lines.
func lineExistsInPatch(file map[string]any, target int) bool {
	if target < 1 || file == nil {
		return false
	}
	patch, _ := file["patch"].(string)
	currentLine := 0
	insideHunk := false
	for _, line := range strings.Split(patch, "\n") {
		if match := unifiedDiffHunk.FindStringSubmatch(line); len(match) == 2 {
			currentLine, _ = strconv.Atoi(match[1])
			insideHunk = true
			continue
		}
		if !insideHunk || line == "" || strings.HasPrefix(line, "\\") {
			continue
		}
		switch line[0] {
		case '+':
			if currentLine == target {
				return true
			}
			currentLine++
		case ' ':
			if currentLine == target {
				return true
			}
			currentLine++
		case '-':
			// Removed lines do not exist in the new revision.
		}
	}
	return false
}

func parseFindings(response string) ([]Finding, error) {
	decoder := json.NewDecoder(strings.NewReader(response))
	var findings []Finding
	if err := decoder.Decode(&findings); err != nil {
		return nil, fmt.Errorf("model response must be a JSON finding list: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("model response must contain one JSON finding list")
	}
	if findings == nil {
		return nil, fmt.Errorf("model response must be a JSON finding list")
	}
	return findings, nil
}

func filterFindings(values []any, config map[string]any) ([]Finding, error) {
	minimum, err := configuredSeverity(config, "minimum_severity", "low")
	if err != nil {
		return nil, err
	}
	findings, err := findingLists(values)
	if err != nil {
		return nil, err
	}
	result := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		if severityRank[finding.Severity] >= severityRank[minimum] {
			result = append(result, finding)
		}
	}
	return deduplicateFindings(result), nil
}

func consolidateFindings(values []any) (Review, error) {
	findings, err := findingLists(values)
	if err != nil {
		return Review{}, err
	}
	return Review{Findings: deduplicateFindings(findings)}, nil
}

func formatReview(values []any) (FormattedReview, error) {
	review, ok := firstValue(values).(Review)
	if !ok {
		return FormattedReview{}, fmt.Errorf("format card requires a review input")
	}
	findings := deduplicateFindings(review.Findings)
	formatted := FormattedReview{Findings: findings, Observations: make([]ReviewObservation, 0, len(findings)), Summary: ReviewSummary{Total: len(findings), Event: "COMMENT", Status: "commented"}}
	for _, finding := range findings {
		switch finding.Severity {
		case "low":
			formatted.Summary.Low++
		case "medium":
			formatted.Summary.Medium++
		case "high":
			formatted.Summary.High++
		case "critical":
			formatted.Summary.Critical++
		}
		formatted.Observations = append(formatted.Observations, ReviewObservation{Path: finding.Path, Body: finding.Comment, NewPosition: finding.Line})
	}
	if formatted.Summary.High > 0 || formatted.Summary.Critical > 0 {
		formatted.Summary.Event = "REQUEST_CHANGES"
		formatted.Summary.Status = "changes_requested"
	}
	return formatted, nil
}

func workflowFiles(values []any) ([]map[string]any, error) {
	files := make([]map[string]any, 0)
	for _, value := range values {
		var items []map[string]any
		switch typed := value.(type) {
		case []map[string]any:
			items = typed
		case FileGroup:
			items = typed.Files
		default:
			return nil, fmt.Errorf("card requires fetched files input")
		}
		for _, file := range items {
			filename, ok := file["filename"].(string)
			if !ok || strings.TrimSpace(filename) == "" {
				return nil, fmt.Errorf("fetched file requires filename")
			}
			files = append(files, file)
		}
	}
	return files, nil
}

func configuredExtensions(config map[string]any, key string) (map[string]bool, error) {
	value, exists := config[key]
	if !exists {
		return map[string]bool{}, nil
	}
	var items []string
	switch typed := value.(type) {
	case []string:
		items = typed
	case []any:
		items = make([]string, len(typed))
		for index, item := range typed {
			stringItem, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("filter card config.%s must be a list of extensions", key)
			}
			items[index] = stringItem
		}
	default:
		return nil, fmt.Errorf("filter card config.%s must be a list of extensions", key)
	}
	result := make(map[string]bool, len(items))
	for _, item := range items {
		normalized := normalizedExtension(item)
		if normalized == "" {
			return nil, fmt.Errorf("filter card config.%s contains an invalid extension", key)
		}
		result[normalized] = true
	}
	return result, nil
}

func configuredBool(config map[string]any, key string, fallback bool) (bool, error) {
	value, exists := config[key]
	if !exists {
		return fallback, nil
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("card config.%s must be a boolean", key)
	}
	return result, nil
}

func configuredSeverity(config map[string]any, key, fallback string) (string, error) {
	value, exists := config[key]
	if !exists {
		return fallback, nil
	}
	severity, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("response_filter card config.%s must be a severity", key)
	}
	if _, allowed := severityRank[severity]; !allowed {
		return "", fmt.Errorf("response_filter card config.%s %q is not allowed", key, severity)
	}
	return severity, nil
}

func normalizedExtension(filename string) string {
	extension := strings.ToLower(path.Ext(strings.TrimSpace(filename)))
	if extension == "" && !strings.Contains(filename, "/") {
		extension = strings.ToLower(strings.TrimSpace(filename))
		if extension != "" && !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
	}
	return extension
}

func generatedFile(file map[string]any) bool {
	if generated, _ := file["generated"].(bool); generated {
		return true
	}
	if generated, _ := file["is_generated"].(bool); generated {
		return true
	}
	filename := strings.ToLower(file["filename"].(string))
	base := path.Base(filename)
	for _, segment := range strings.Split(filename, "/") {
		if segment == "vendor" || segment == "node_modules" || segment == "dist" || segment == "build" || segment == "coverage" {
			return true
		}
	}
	return strings.Contains(base, ".min.") || strings.Contains(base, ".generated.") || strings.Contains(base, "_generated.") || strings.HasSuffix(base, ".pb.go") || base == "package-lock.json" || base == "yarn.lock" || base == "pnpm-lock.yaml" || base == "go.sum"
}

func fileCharacters(file map[string]any) int {
	for _, key := range []string{"patch", "content", "diff"} {
		if value, ok := file[key].(string); ok {
			return utf8.RuneCountInString(value)
		}
	}
	return utf8.RuneCountInString(file["filename"].(string))
}

func sortFiles(files []map[string]any) {
	sort.SliceStable(files, func(i, j int) bool {
		return files[i]["filename"].(string) < files[j]["filename"].(string)
	})
}

func findingLists(values []any) ([]Finding, error) {
	findings := make([]Finding, 0)
	var appendValue func(any) error
	appendValue = func(value any) error {
		switch items := value.(type) {
		case []Finding:
			findings = append(findings, items...)
			return nil
		case []any:
			for _, item := range items {
				if err := appendValue(item); err != nil {
					return err
				}
			}
			return nil
		default:
			return fmt.Errorf("card requires validated finding lists")
		}
	}
	for _, value := range values {
		if err := appendValue(value); err != nil {
			return nil, err
		}
	}
	return findings, nil
}

func deduplicateFindings(findings []Finding) []Finding {
	unique := make(map[string]Finding, len(findings))
	for _, finding := range findings {
		key := finding.Path + "\x00" + fmt.Sprint(finding.Line) + "\x00" + finding.Comment + "\x00" + finding.Severity
		unique[key] = finding
	}
	result := make([]Finding, 0, len(unique))
	for _, finding := range unique {
		result = append(result, finding)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		if result[i].Severity != result[j].Severity {
			return severityRank[result[i].Severity] > severityRank[result[j].Severity]
		}
		return result[i].Comment < result[j].Comment
	})
	return result
}

func firstValue(values []any) any {
	if len(values) > 0 {
		return values[0]
	}
	return nil
}
