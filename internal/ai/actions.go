package ai

import (
	"encoding/json"
	"strings"
)

// ParseActions extracts pry-action blocks from agent response text.
// Returns the actions found and the text with action blocks removed.
func ParseActions(text string) ([]Action, string) {
	var actions []Action
	cleaned := text

	for {
		start := strings.Index(cleaned, "```pry-action\n")
		if start < 0 {
			break
		}
		end := strings.Index(cleaned[start+14:], "\n```")
		if end < 0 {
			break
		}
		end += start + 14

		jsonStr := strings.TrimSpace(cleaned[start+14 : end])

		var action Action
		if err := json.Unmarshal([]byte(jsonStr), &action); err == nil {
			if action.Action != "" && action.Path != "" && action.Line > 0 {
				actions = append(actions, action)
			}
		}

		// Remove the action block from the text
		cleaned = cleaned[:start] + cleaned[end+4:]
	}

	return actions, strings.TrimSpace(cleaned)
}
