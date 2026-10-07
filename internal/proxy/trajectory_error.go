package proxy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Matches:
	// "API error (attempt 1): request failed: ..."
	// "API error (attempt 1 of 9): request failed: ..."
	// "(attempt 2/9): ..."
	// "Error (attempt 3): ..."
	attemptRegex = regexp.MustCompile(`(?i)^(.*?)\s*\(attempt\s+(\d+)(?:\s*(?:of|/)\s*(\d+))?\)\s*[:：\-]?\s*(.*)$`)

	// Matches formatted attempt string e.g. "API error (attempt 2/9 · 已重试 2 次): ..."
	attemptMergedRegex = regexp.MustCompile(`(?i)^(.*?)\s*\(attempt\s+(\d+)(?:\s*(?:of|/)\s*(\d+))?(?:\s*[·,]\s*[^)]*)?\)\s*[:：\-]?\s*(.*)$`)
)

type parsedAttemptError struct {
	isAttempt   bool
	prefix      string // e.g., "API error"
	attempt     int    // e.g., 1
	maxAttempts int    // e.g., 9
	baseError   string // e.g., "request failed: Post ..."
}

func parseAttemptError(text string) parsedAttemptError {
	trimmed := strings.TrimSpace(text)
	m := attemptMergedRegex.FindStringSubmatch(trimmed)
	if len(m) == 0 {
		m = attemptRegex.FindStringSubmatch(trimmed)
	}
	if len(m) > 0 {
		prefix := strings.TrimSpace(m[1])
		if prefix == "" {
			prefix = "API error"
		}
		attempt, _ := strconv.Atoi(m[2])
		maxAttempts := 0
		if m[3] != "" {
			maxAttempts, _ = strconv.Atoi(m[3])
		}
		if maxAttempts == 0 {
			// Client retries 9 times by default for API errors
			if attempt <= 9 {
				maxAttempts = 9
			} else {
				maxAttempts = attempt
			}
		}
		baseError := strings.TrimSpace(m[4])
		return parsedAttemptError{
			isAttempt:   true,
			prefix:      prefix,
			attempt:     attempt,
			maxAttempts: maxAttempts,
			baseError:   baseError,
		}
	}

	return parsedAttemptError{
		isAttempt: false,
		baseError: trimmed,
	}
}

func formatAttemptError(prefix string, attempt, maxAttempts int, baseError string) string {
	if prefix == "" {
		prefix = "API error"
	}
	if maxAttempts > 0 {
		if attempt > 1 {
			return fmt.Sprintf("%s (attempt %d/%d · 已重试 %d 次): %s", prefix, attempt, maxAttempts, attempt, baseError)
		}
		return fmt.Sprintf("%s (attempt %d/%d): %s", prefix, attempt, maxAttempts, baseError)
	}
	if attempt > 1 {
		return fmt.Sprintf("%s (attempt %d · 已重试 %d 次): %s", prefix, attempt, attempt, baseError)
	}
	return fmt.Sprintf("%s (attempt %d): %s", prefix, attempt, baseError)
}

func tryMergeAttemptErrors(prev CascadeMessageItem, currRawText string) (CascadeMessageItem, bool) {
	if prev.Type != "error" {
		return prev, false
	}

	prevParsed := parseAttemptError(prev.Text)
	currParsed := parseAttemptError(currRawText)

	// Case 1: Both are attempt-based errors with matching base error
	if prevParsed.isAttempt && currParsed.isAttempt {
		if normalizeErrKey(prevParsed.baseError) == normalizeErrKey(currParsed.baseError) {
			newAttempt := currParsed.attempt
			if newAttempt <= prevParsed.attempt {
				newAttempt = prevParsed.attempt + 1
			}
			maxAttempts := currParsed.maxAttempts
			if maxAttempts == 0 {
				maxAttempts = prevParsed.maxAttempts
			}
			if maxAttempts == 0 {
				maxAttempts = 9
			}

			formatted := formatAttemptError(currParsed.prefix, newAttempt, maxAttempts, currParsed.baseError)
			prev.Text = formatted
			prev.Content = formatted
			prev.AttemptCount = newAttempt
			prev.MaxAttempts = maxAttempts
			return prev, true
		}
	}

	// Case 2: Curr is attempt error, but prev was exact same base error without attempt marker yet
	if currParsed.isAttempt && normalizeErrKey(prev.Text) == normalizeErrKey(currParsed.baseError) {
		newAttempt := currParsed.attempt
		if newAttempt < 2 {
			newAttempt = 2
		}
		maxAttempts := currParsed.maxAttempts
		if maxAttempts == 0 {
			maxAttempts = 9
		}
		formatted := formatAttemptError(currParsed.prefix, newAttempt, maxAttempts, currParsed.baseError)
		prev.Text = formatted
		prev.Content = formatted
		prev.AttemptCount = newAttempt
		prev.MaxAttempts = maxAttempts
		return prev, true
	}

	// Case 3: Both are non-attempt errors, but have identical normalized text (consecutive duplicate errors)
	if !prevParsed.isAttempt && !currParsed.isAttempt {
		if normalizeErrKey(prev.Text) == normalizeErrKey(currRawText) {
			curCount := prev.AttemptCount
			if curCount < 1 {
				curCount = 1
			}
			curCount++
			prev.AttemptCount = curCount
			prev.Text = fmt.Sprintf("%s (已重试 %d 次)", strings.TrimSpace(currRawText), curCount)
			prev.Content = prev.Text
			return prev, true
		}
	}

	return prev, false
}

func normalizeErrKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	// Strip trailing punctuation
	s = strings.TrimRight(s, ".:;, \t\r\n")
	return s
}
