package workflow

import (
	"crypto/sha256"
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
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Comment     string `json:"comment"`
	Severity    string `json:"severity"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

const (
	CandidatePending       = "PENDING"
	CandidateConfirmed     = "CONFIRMED"
	CandidateRejected      = "REJECTED"
	CandidateNotApplicable = "NOT_APPLICABLE"
	CandidateNeedsContext  = "NEEDS_CONTEXT"
	CandidateNotObservable = "NOT_OBSERVABLE"
)

// CandidateFinding is an internal proposal. It is never publishable until an
// independent validator marks it CONFIRMED and converts it to Finding.
type CandidateFinding struct {
	CheckID             string   `json:"check_id"`
	Claim               string   `json:"claim"`
	Scenario            string   `json:"scenario"`
	Impact              string   `json:"impact"`
	Evidence            []string `json:"evidence"`
	Confidence          float64  `json:"confidence"`
	RequiredContext     []string `json:"required_context"`
	Symbol              string   `json:"symbol"`
	IssueType           string   `json:"issue_type"`
	AffectedEntity      string   `json:"affected_entity"`
	Path                string   `json:"path"`
	Line                int      `json:"line"`
	Comment             string   `json:"comment"`
	Severity            string   `json:"severity"`
	Status              string   `json:"status"`
	DecisionReason      string   `json:"decision_reason,omitempty"`
	Fingerprint         string   `json:"fingerprint,omitempty"`
	ValidationAttempted bool     `json:"validation_attempted,omitempty"`
}

type CandidateValidationDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type ValidationFailure struct {
	Code   string   `json:"code,omitempty"`
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
	responseValue := firstValue(responseValues)
	response, ok := responseValue.(string)
	var contract ReviewContractVersion
	if typed, typedOK := responseValue.(ReviewModelResponse); typedOK {
		response, contract, ok = typed.Content, typed.Contract, true
	}
	if !ok {
		return ValidationFailure{Code: "invalid_json", Errors: []string{"model response must be JSON text"}}, "invalid"
	}
	schemaValue, hasSchema := responseSchemaFromConfig(config)
	if contractConfigured(contract) {
		schemaValue, hasSchema = contract.ResponseSchema, true
	}
	if hasSchema {
		parsed, err := parseJSONResponse(response)
		if err != nil {
			return ValidationFailure{Code: "invalid_json", Errors: []string{err.Error()}}, "invalid"
		}
		schema, err := validateResponseSchema(schemaValue)
		if err != nil {
			return ValidationFailure{Code: "invalid_schema", Errors: []string{err.Error()}}, "invalid"
		}
		if err = schema.Validate(parsed); err != nil {
			return ValidationFailure{Code: "schema_mismatch", Errors: []string{err.Error()}}, "invalid"
		}
		validatePaths, configErr := configuredBool(config, "validate_paths", false)
		if configErr != nil {
			return ValidationFailure{Code: "additional_rule", Errors: []string{configErr.Error()}}, "invalid"
		}
		if !validatePaths {
			if contractConfigured(contract) {
				return ValidatedReviewResponse{Value: parsed, Contract: contract}, "valid"
			}
			return parsed, "valid"
		}
		findings, findingErr := findingsFromValue(parsed)
		if findingErr != nil {
			return ValidationFailure{Code: "additional_rule", Errors: []string{"config.validate_paths requires a Finding[] compatible response"}}, "invalid"
		}
		validated, port := validateFindingRules(findings, fileValues, true)
		if port == "invalid" {
			return validated, port
		}
		if contractConfigured(contract) {
			return ValidatedReviewResponse{Value: parsed, Contract: contract}, "valid"
		}
		return parsed, "valid"
	}
	findings, err := parseFindings(response)
	if err != nil {
		return ValidationFailure{Code: "invalid_json", Errors: []string{err.Error()}}, "invalid"
	}
	validatePaths, err := configuredBool(config, "validate_paths", false)
	if err != nil {
		return ValidationFailure{Code: "additional_rule", Errors: []string{err.Error()}}, "invalid"
	}
	return validateFindingRules(findings, fileValues, validatePaths)
}

func candidateFindings(values []any) ([]CandidateFinding, error) {
	candidates, _, err := candidateFindingsWithContract(values)
	return candidates, err
}

func candidateFindingsWithContract(values []any) ([]CandidateFinding, ReviewContractVersion, error) {
	var candidates []CandidateFinding
	var contract ReviewContractVersion
	for _, value := range values {
		switch typed := value.(type) {
		case ValidatedReviewResponse:
			if contractConfigured(contract) && (contract.Key != typed.Contract.Key || contract.Version != typed.Contract.Version) {
				return nil, ReviewContractVersion{}, fmt.Errorf("candidate validator received mixed review contract versions")
			}
			contract = typed.Contract
			decoded, _, err := candidateFindingsWithContract([]any{typed.Value})
			if err != nil {
				return nil, ReviewContractVersion{}, err
			}
			candidates = append(candidates, decoded...)
		case []CandidateFinding:
			candidates = append(candidates, typed...)
		case CandidateFinding:
			candidates = append(candidates, typed)
		default:
			payload, err := json.Marshal(value)
			if err != nil {
				return nil, ReviewContractVersion{}, fmt.Errorf("candidate validator requires CandidateFinding[]")
			}
			var decoded []CandidateFinding
			if err = json.Unmarshal(payload, &decoded); err != nil || decoded == nil {
				return nil, ReviewContractVersion{}, fmt.Errorf("candidate validator requires CandidateFinding[]")
			}
			candidates = append(candidates, decoded...)
		}
	}
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.Status == "" {
			candidate.Status = CandidatePending
		}
		if candidate.Status != CandidatePending {
			return nil, ReviewContractVersion{}, fmt.Errorf("candidate %d must enter validation with status PENDING", index)
		}
		if strings.TrimSpace(candidate.CheckID) == "" ||
			strings.TrimSpace(candidate.Claim) == "" ||
			strings.TrimSpace(candidate.Scenario) == "" ||
			strings.TrimSpace(candidate.Impact) == "" ||
			strings.TrimSpace(candidate.Symbol) == "" ||
			strings.TrimSpace(candidate.IssueType) == "" ||
			strings.TrimSpace(candidate.AffectedEntity) == "" ||
			strings.TrimSpace(candidate.Path) == "" ||
			candidate.Line < 1 ||
			strings.TrimSpace(candidate.Comment) == "" ||
			len(candidate.Evidence) == 0 ||
			candidate.Confidence < 0 || candidate.Confidence > 1 {
			return nil, ReviewContractVersion{}, fmt.Errorf("candidate %d does not satisfy the CandidateFinding contract", index)
		}
		if _, allowed := severityRank[candidate.Severity]; !allowed {
			return nil, ReviewContractVersion{}, fmt.Errorf("candidate %d severity %q is not allowed", index, candidate.Severity)
		}
		for _, evidence := range candidate.Evidence {
			if strings.TrimSpace(evidence) == "" {
				return nil, ReviewContractVersion{}, fmt.Errorf("candidate %d contains empty evidence", index)
			}
		}
	}
	return candidates, contract, nil
}

func parseCandidateDecision(response string) (CandidateValidationDecision, error) {
	decoder := json.NewDecoder(strings.NewReader(response))
	decoder.DisallowUnknownFields()
	var decision CandidateValidationDecision
	if err := decoder.Decode(&decision); err != nil {
		return decision, fmt.Errorf("candidate validator response must be a decision object: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return decision, fmt.Errorf("candidate validator response must contain one decision object")
	}
	if decision.Decision != CandidateConfirmed && decision.Decision != CandidateRejected && decision.Decision != CandidateNeedsContext {
		return decision, fmt.Errorf("candidate validator decision must be CONFIRMED, REJECTED, or NEEDS_CONTEXT")
	}
	if strings.TrimSpace(decision.Reason) == "" {
		return decision, fmt.Errorf("candidate validator decision reason is required")
	}
	return decision, nil
}

func confirmedFinding(candidate CandidateFinding) Finding {
	return Finding{Path: candidate.Path, Line: candidate.Line, Comment: candidate.Comment, Severity: candidate.Severity, Fingerprint: candidate.Fingerprint}
}

func candidateFingerprint(candidate CandidateFinding, file map[string]any) string {
	parts := []string{
		systemFileIdentity(file, "_forgereview_repository", "unknown"),
		systemFileIdentity(file, "_forgereview_base_commit", "unknown"),
		strings.TrimSpace(candidate.Path),
		normalizedIdentityPart(semanticFingerprintSymbol(candidate, file)),
		normalizedIdentityPart(candidate.CheckID),
		normalizedIdentityPart(candidate.IssueType),
		normalizedIdentityPart(candidate.AffectedEntity),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("sha256:%x", sum[:])
}

func semanticFingerprintSymbol(candidate CandidateFinding, file map[string]any) string {
	if symbol, _ := file["_forgereview_unit_symbol"].(string); strings.TrimSpace(symbol) != "" {
		return strings.TrimSpace(symbol)
	}
	return strings.TrimSpace(candidate.Symbol)
}

func normalizedIdentityPart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func systemFileIdentity(file map[string]any, key, fallback string) string {
	value, _ := file[key].(string)
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func validateFindingRules(findings []Finding, fileValues []any, validatePaths bool) (any, string) {
	var filesByPath map[string]map[string]any
	if validatePaths {
		if len(fileValues) == 0 {
			return ValidationFailure{Code: "additional_rule", Errors: []string{"validate card requires fetched files when config.validate_paths is true"}}, "invalid"
		}
		files, fileErr := workflowFiles(fileValues)
		if fileErr != nil {
			return ValidationFailure{Code: "additional_rule", Errors: []string{"validate card requires fetched files when config.validate_paths is true"}}, "invalid"
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
		} else if validatePaths && !lineIsAddedInPatch(filesByPath[finding.Path], finding.Line) {
			errors = append(errors, fmt.Sprintf("finding %d line %d is not an added line in %q", index, finding.Line, finding.Path))
		}
	}
	if len(errors) > 0 {
		return ValidationFailure{Code: "additional_rule", Errors: errors}, "invalid"
	}
	return findings, "valid"
}

func parseJSONResponse(response string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(response))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("model response must be valid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("model response must contain one JSON value")
	}
	return value, nil
}

func findingsFromValue(value any) ([]Finding, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return parseFindings(string(payload))
}

var unifiedDiffHunk = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// lineIsAddedInPatch accepts only a line introduced by the pull request.
// Context lines are valid Gitea anchors, but accepting them lets a model attach
// an observation to a nearby comment or brace instead of the code it discusses.
func lineIsAddedInPatch(file map[string]any, target int) bool {
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
		case SemanticUnit:
			if typed.file == nil {
				return nil, fmt.Errorf("semantic unit requires observed file context")
			}
			items = []map[string]any{typed.file}
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

func reviewableFileCount(files []map[string]any) int {
	count := 0
	for _, file := range files {
		for _, key := range []string{"patch", "content", "diff"} {
			if value, ok := file[key].(string); ok && strings.TrimSpace(value) != "" {
				count++
				break
			}
		}
	}
	return count
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
		key := finding.Fingerprint
		if key == "" {
			key = finding.Path + "\x00" + fmt.Sprint(finding.Line) + "\x00" + finding.Comment + "\x00" + finding.Severity
		}
		if _, exists := unique[key]; !exists {
			unique[key] = finding
		}
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
