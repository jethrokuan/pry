package diffview

import (
	"strings"

	"charm.land/bubbles/v2/textarea"

	"github.com/jethrokuan/pry/internal/review"
)

// mentionTrigger extracts the @prefix being typed at the cursor position.
// Returns the prefix (without @) and the byte offset of the @ character,
// or ("", -1) if no mention is being typed.
func mentionTrigger(ta textarea.Model) (string, int) {
	value := ta.Value()
	if value == "" {
		return "", -1
	}

	// Compute absolute cursor offset: sum of all lines before current + current column
	lines := strings.Split(value, "\n")
	curLine := ta.Line()
	if curLine >= len(lines) {
		return "", -1
	}
	li := ta.LineInfo()
	col := li.CharOffset

	offset := 0
	for i := 0; i < curLine; i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}
	offset += col

	// Scan backwards from cursor to find @
	text := value[:offset]
	atIdx := -1
	for i := len(text) - 1; i >= 0; i-- {
		ch := text[i]
		if ch == '@' {
			// @ must be at start of line or preceded by whitespace
			if i == 0 || text[i-1] == ' ' || text[i-1] == '\t' || text[i-1] == '\n' {
				atIdx = i
			}
			break
		}
		// Stop if we hit whitespace (no @ mention in progress)
		if ch == ' ' || ch == '\t' || ch == '\n' {
			break
		}
	}

	if atIdx < 0 {
		return "", -1
	}

	prefix := text[atIdx+1:]
	return prefix, atIdx
}

// filterMentionUsers returns users matching the prefix by login or name (case-insensitive).
func filterMentionUsers(users []review.MentionableUser, prefix string) []review.MentionableUser {
	if len(users) == 0 {
		return nil
	}
	prefix = strings.ToLower(prefix)
	var matches []review.MentionableUser
	for _, u := range users {
		if strings.HasPrefix(strings.ToLower(u.Login), prefix) ||
			(u.Name != "" && strings.HasPrefix(strings.ToLower(u.Name), prefix)) {
			matches = append(matches, u)
		}
	}
	return matches
}
