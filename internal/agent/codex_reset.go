package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Match the retry instruction only beside a usage-limit error, never a /status
// quota window or an arbitrary time in ordinary terminal output.
var codexRetryPattern = regexp.MustCompile(`(?i)(?:usage limit|usage cap|rate limit)[\s\S]{0,600}?try again at\s+((?:(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}(?:st|nd|rd|th)?[,]?\s+\d{4}[,]?\s+)?\d{1,2}[:h]\d{2}(?:\s*[AP]M)?)\b`)
var clockSuffixPattern = regexp.MustCompile(`(?i)(\d)(AM|PM)\b`)
var ordinalDayPattern = regexp.MustCompile(`(?i)(\d)(st|nd|rd|th)\b`)

func codexResetText(screen string) string {
	screen = ansiPattern.ReplaceAllString(screen, "")
	lines := strings.Split(screen, "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	// Terminal wrapping may split the instruction or its date across lines.
	screen = strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
	matches := codexRetryPattern.FindAllStringSubmatch(screen, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

func parseCodexReset(text string, now time.Time) int64 {
	normalized := strings.ToUpper(ordinalDayPattern.ReplaceAllString(text, "$1"))
	normalized = strings.ReplaceAll(normalized, ",", "")
	normalized = strings.ReplaceAll(normalized, "H", ":")
	normalized = clockSuffixPattern.ReplaceAllString(normalized, "$1 $2")
	normalized = strings.Join(strings.Fields(normalized), " ")
	// Codex renders explicit dates for a reset on a different calendar day.
	for _, layout := range []string{"Jan 2 2006 3:04 PM", "Jan 2 2006 15:04"} {
		if reset, err := time.ParseInLocation(layout, normalized, now.Location()); err == nil {
			return reset.Unix()
		}
	}
	parts := strings.Fields(normalized)
	if len(parts) == 0 || len(parts) > 2 {
		return 0
	}
	clock := strings.Split(parts[0], ":")
	if len(clock) != 2 {
		return 0
	}
	hour, e1 := strconv.Atoi(clock[0])
	minute, e2 := strconv.Atoi(clock[1])
	if e1 != nil || e2 != nil || minute < 0 || minute > 59 {
		return 0
	}
	if len(parts) == 2 {
		if hour < 1 || hour > 12 || (parts[1] != "AM" && parts[1] != "PM") {
			return 0
		}
		hour %= 12
		if parts[1] == "PM" {
			hour += 12
		}
	} else if hour < 0 || hour > 23 {
		return 0
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location()).Unix()
}

func (t *screenTracker) codexReset(name, screen string, now time.Time) int64 {
	text := codexResetText(screen)
	t.mu.Lock()
	defer t.mu.Unlock()
	previous := t.seen[name]
	if text != previous.resetText {
		previous.resetText = text
		previous.resetAt = parseCodexReset(text, now)
		t.seen[name] = previous
	}
	// Resolve an unchanged message once, so yesterday's clock cannot become
	// today's suggestion. A suggestion expires after its one-minute grace period.
	if previous.resetAt+60 <= now.Unix() || previous.resetAt > now.Add(30*24*time.Hour).Unix() {
		return 0
	}
	return previous.resetAt
}
