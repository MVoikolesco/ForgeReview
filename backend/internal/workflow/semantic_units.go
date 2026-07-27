package workflow

import (
	"crypto/sha256"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type SemanticUnit struct {
	UnitID           string   `json:"unit_id"`
	Repository       string   `json:"repository,omitempty"`
	BaseCommit       string   `json:"base_commit,omitempty"`
	Path             string   `json:"path"`
	Language         string   `json:"language"`
	Kind             string   `json:"kind"`
	Symbol           string   `json:"symbol"`
	StartLine        int      `json:"start_line"`
	EndLine          int      `json:"end_line"`
	AddedLines       []int    `json:"added_lines"`
	Diff             string   `json:"diff"`
	ContextLines     []string `json:"context_lines"`
	Imports          []string `json:"imports"`
	Dependencies     []string `json:"dependencies"`
	RelatedTests     []string `json:"related_tests"`
	RelatedContracts []string `json:"related_contracts"`
	AvailableContext []string `json:"available_context"`
	file             map[string]any
}

type semanticUnitSettings struct {
	MaxUnits      int
	MaxCharacters int
	ContextLines  int
}

var semanticHunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@\s*(.*)$`)
var quotedDependency = regexp.MustCompile(`["']([^"']+)["']`)

func semanticUnitSettingsFor(node Node) (semanticUnitSettings, error) {
	settings := semanticUnitSettings{MaxUnits: 200, MaxCharacters: 50000, ContextLines: 4}
	for key, bounds := range map[string]struct {
		target       *int
		minimum, max int
	}{
		"max_units":      {&settings.MaxUnits, 1, 1000},
		"max_characters": {&settings.MaxCharacters, 1000, 200000},
		"context_lines":  {&settings.ContextLines, 0, 20},
	} {
		value, exists := node.Config[key]
		if !exists {
			continue
		}
		number, ok := integer(value)
		if !ok || number < bounds.minimum || number > bounds.max {
			return semanticUnitSettings{}, fmt.Errorf("semantic_units card %q config.%s must be between %d and %d", node.Key, key, bounds.minimum, bounds.max)
		}
		*bounds.target = number
	}
	return settings, nil
}

func buildSemanticUnits(values []any, node Node) ([]SemanticUnit, error) {
	settings, err := semanticUnitSettingsFor(node)
	if err != nil {
		return nil, err
	}
	files, err := workflowFiles(values)
	if err != nil {
		return nil, err
	}
	sortFiles(files)
	testPaths, contractPaths := relatedChangedPaths(files)
	units := make([]SemanticUnit, 0)
	for _, file := range files {
		patchText, _ := file["patch"].(string)
		hunks := semanticHunks(patchText)
		if len(hunks) == 0 {
			continue
		}
		for _, hunk := range hunks {
			if utf8.RuneCountInString(hunk) > settings.MaxCharacters {
				return nil, fmt.Errorf("semantic unit for %q exceeds config.max_characters", file["filename"])
			}
			unit, buildErr := semanticUnitForHunk(file, hunk, settings.ContextLines, testPaths, contractPaths)
			if buildErr != nil {
				return nil, buildErr
			}
			units = append(units, unit)
			if len(units) > settings.MaxUnits {
				return nil, fmt.Errorf("semantic_units card %q produced more than config.max_units=%d", node.Key, settings.MaxUnits)
			}
		}
	}
	return units, nil
}

func semanticHunks(patchText string) []string {
	lines := strings.Split(strings.ReplaceAll(patchText, "\r\n", "\n"), "\n")
	starts := make([]int, 0)
	for index, line := range lines {
		if semanticHunkHeader.MatchString(line) {
			starts = append(starts, index)
		}
	}
	hunks := make([]string, 0, len(starts))
	for index, start := range starts {
		end := len(lines)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		hunks = append(hunks, strings.Join(lines[start:end], "\n"))
	}
	return hunks
}

func semanticUnitForHunk(file map[string]any, hunk string, contextLimit int, testPaths, contractPaths []string) (SemanticUnit, error) {
	lines := strings.Split(hunk, "\n")
	if len(lines) == 0 {
		return SemanticUnit{}, fmt.Errorf("semantic unit requires a diff hunk")
	}
	header := semanticHunkHeader.FindStringSubmatch(lines[0])
	if len(header) != 4 {
		return SemanticUnit{}, fmt.Errorf("semantic unit requires a valid diff hunk header")
	}
	start, _ := strconv.Atoi(header[1])
	count := 1
	if header[2] != "" {
		count, _ = strconv.Atoi(header[2])
	}
	filename := file["filename"].(string)
	language := semanticLanguage(filename)
	symbol, kind := semanticSymbol(strings.TrimSpace(header[3]), language, lines[1:])
	if symbol == "" {
		symbol, kind = "<file>", "hunk"
	}
	added, contextLines := semanticLines(start, lines[1:], contextLimit)
	imports := semanticImports(language, lines[1:])
	repository, _ := file["_forgereview_repository"].(string)
	baseCommit, _ := file["_forgereview_base_commit"].(string)
	relatedTests := relatedForPath(filename, testPaths)
	relatedContracts := relatedForPath(filename, contractPaths)
	available := []string{ContextDiff, ContextFile}
	if kind == "symbol" {
		available = append(available, ContextSymbol)
	}
	if len(imports) > 0 {
		available = append(available, ContextDependencies)
	}
	if len(relatedTests) > 0 {
		available = append(available, ContextTests)
	}
	if len(relatedContracts) > 0 {
		available = append(available, ContextContracts)
	}
	if repository != "" {
		available = append(available, ContextRepository)
	}
	unitID := semanticUnitID(repository, baseCommit, filename, symbol, start)
	scopedFile := make(map[string]any, len(file)+1)
	for key, value := range file {
		scopedFile[key] = value
	}
	scopedFile["patch"] = hunk
	scopedFile["_forgereview_unit_id"] = unitID
	scopedFile["_forgereview_unit_symbol"] = symbol
	scopedFile["_forgereview_available_context"] = append([]string(nil), available...)
	return SemanticUnit{
		UnitID: unitID, Repository: repository, BaseCommit: baseCommit,
		Path: filename, Language: language, Kind: kind, Symbol: symbol,
		StartLine: start, EndLine: start + max(count-1, 0), AddedLines: added,
		Diff: hunk, ContextLines: contextLines, Imports: imports, Dependencies: append([]string(nil), imports...),
		RelatedTests: relatedTests, RelatedContracts: relatedContracts, AvailableContext: available,
		file: scopedFile,
	}, nil
}

func semanticLines(start int, lines []string, contextLimit int) ([]int, []string) {
	current := start
	added := make([]int, 0)
	contextLines := make([]string, 0)
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "\\") {
			continue
		}
		switch line[0] {
		case '+':
			added = append(added, current)
			current++
		case ' ':
			if len(contextLines) < contextLimit {
				contextLines = append(contextLines, strings.TrimPrefix(line, " "))
			}
			current++
		case '-':
			if len(contextLines) < contextLimit {
				contextLines = append(contextLines, strings.TrimPrefix(line, "-"))
			}
		}
	}
	return added, contextLines
}

func semanticSymbol(header, language string, lines []string) (string, string) {
	candidates := make([]string, 0, len(lines)+1)
	if header != "" {
		candidates = append(candidates, header)
	}
	for _, line := range lines {
		if len(line) > 1 && (line[0] == '+' || line[0] == ' ' || line[0] == '-') {
			candidates = append(candidates, strings.TrimSpace(line[1:]))
		}
	}
	patterns := semanticSymbolPatterns(language)
	for _, candidate := range candidates {
		for _, pattern := range patterns {
			if match := pattern.FindStringSubmatch(candidate); len(match) > 1 {
				return match[1], "symbol"
			}
		}
	}
	if header != "" {
		header = strings.Join(strings.Fields(header), " ")
		if len([]rune(header)) > 160 {
			header = string([]rune(header)[:160])
		}
		return header, "symbol"
	}
	return "", "hunk"
}

func semanticSymbolPatterns(language string) []*regexp.Regexp {
	common := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)`),
		regexp.MustCompile(`(?i)^(?:export\s+)?class\s+([A-Za-z_$][\w$]*)`),
		regexp.MustCompile(`(?i)^(?:public|private|protected|static|async|\s)*function\s+([A-Za-z_$][\w$]*)`),
	}
	if language == "go" {
		return []*regexp.Regexp{
			regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`),
			regexp.MustCompile(`^type\s+([A-Za-z_]\w*)\s+`),
		}
	}
	return append(common,
		regexp.MustCompile(`(?i)^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`),
	)
}

func semanticImports(language string, lines []string) []string {
	items := map[string]bool{}
	goImportBlock := false
	for _, raw := range lines {
		if len(raw) < 2 || raw[0] == '-' {
			continue
		}
		line := strings.TrimSpace(raw[1:])
		if language == "go" {
			if strings.HasPrefix(line, "import (") {
				goImportBlock = true
				continue
			}
			if goImportBlock && line == ")" {
				goImportBlock = false
				continue
			}
			if goImportBlock {
				if match := quotedDependency.FindStringSubmatch(line); len(match) == 2 {
					items[match[1]] = true
				}
				continue
			}
		}
		relevant := strings.HasPrefix(line, "import ") || strings.Contains(line, " from ") ||
			strings.Contains(line, "require(") || (language == "php" && strings.HasPrefix(line, "use "))
		if !relevant {
			continue
		}
		if match := quotedDependency.FindStringSubmatch(line); len(match) == 2 {
			items[match[1]] = true
		} else if language == "php" && strings.HasPrefix(line, "use ") {
			items[strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "use ")), ";")] = true
		} else if strings.HasPrefix(line, "import ") {
			value := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "import ")), ";")
			if value != "" && value != "(" {
				items[value] = true
			}
		}
	}
	result := make([]string, 0, len(items))
	for item := range items {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func semanticLanguage(filename string) string {
	switch normalizedExtension(filename) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".php":
		return "php"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	default:
		return strings.TrimPrefix(normalizedExtension(filename), ".")
	}
}

func semanticUnitID(repository, baseCommit, filename, symbol string, start int) string {
	payload := strings.Join([]string{
		strings.TrimSpace(repository), strings.TrimSpace(baseCommit),
		strings.TrimSpace(filename), strings.TrimSpace(symbol), strconv.Itoa(start),
	}, "\x00")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func relatedChangedPaths(files []map[string]any) ([]string, []string) {
	tests := make([]string, 0)
	contracts := make([]string, 0)
	for _, file := range files {
		filename := file["filename"].(string)
		lower := strings.ToLower(filename)
		base := strings.ToLower(path.Base(filename))
		if strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
			strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") {
			tests = append(tests, filename)
		}
		if strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger") || strings.Contains(lower, "schema") ||
			strings.Contains(lower, "contract") || path.Ext(lower) == ".proto" {
			contracts = append(contracts, filename)
		}
	}
	sort.Strings(tests)
	sort.Strings(contracts)
	return tests, contracts
}

func relatedForPath(filename string, candidates []string) []string {
	directory := path.Dir(filename)
	stem := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
	result := make([]string, 0)
	for _, candidate := range candidates {
		candidateStem := strings.TrimSuffix(path.Base(candidate), path.Ext(candidate))
		candidateStem = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(candidateStem, "_test"), ".test"), ".spec")
		if path.Dir(candidate) == directory || candidateStem == stem {
			result = append(result, candidate)
		}
	}
	return result
}

func semanticUnitFromValues(values []any) (SemanticUnit, bool) {
	for _, value := range values {
		if unit, ok := value.(SemanticUnit); ok {
			return unit, true
		}
	}
	return SemanticUnit{}, false
}
