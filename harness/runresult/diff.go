package main

import (
	"bufio"
	"os"
	"strings"
)

// classifyPatch splits final.patch line counts into contract (formal inputs
// under api/ and db/), generated (frozen Codegen outputs), and handwritten
// surfaces. The path classification rules are draft until the analysis freeze.
func classifyPatch(path, treatment string) (DiffLines, error) {
	var diff DiffLines
	file, err := os.Open(path)
	if err != nil {
		return diff, err
	}
	defer file.Close()

	current := ""
	account := func() {
		if current == "" {
			return
		}
		switch classifyPath(current, treatment) {
		case "contract":
			diff.ContractLines++
		case "generated":
			diff.GeneratedLines++
		default:
			diff.HandwrittenLines++
		}
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	oldPath := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "--- "):
			oldPath = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "---")), "a/")
		case strings.HasPrefix(line, "+++ "):
			target := strings.TrimSpace(strings.TrimPrefix(line, "+++"))
			if target == "/dev/null" {
				current = oldPath
			} else {
				current = strings.TrimPrefix(target, "b/")
			}
		case strings.HasPrefix(line, "+"):
			account()
		case strings.HasPrefix(line, "-"):
			account()
		}
	}
	return diff, scanner.Err()
}

func classifyPath(path, treatment string) string {
	switch {
	case strings.HasPrefix(path, "api/"), strings.HasPrefix(path, "db/"):
		return "contract"
	case isGeneratedPath(path, treatment):
		return "generated"
	default:
		return "handwritten"
	}
}
