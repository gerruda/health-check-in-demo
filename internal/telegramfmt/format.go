// Package telegramfmt превращает Markdown в безопасный Telegram-HTML.
// Telegram не поддерживает таблицы, поэтому таблицы становятся списками.
package telegramfmt

import (
	"regexp"
	"strings"
)

// ParseMode — режим parse_mode для SendMessage.
const ParseMode = "HTML"

var (
	headingPattern  = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	listPattern     = regexp.MustCompile(`^\s{0,3}[-*•]\s+(.+)$`)
	orderedPattern  = regexp.MustCompile(`^\s{0,3}(\d{1,2})[.)]\s+(.+)$`)
	horizontalRule  = regexp.MustCompile(`^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$`)
	boldPattern     = regexp.MustCompile(`\*\*([^*\n]+)\*\*|__([^_\n]+)__`)
	codeSpanPattern = regexp.MustCompile("`([^`\\n]+)`")
	linkPattern     = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	tableSepPattern = regexp.MustCompile(`^:?-{3,}:?$`)
)

// Format конвертирует Markdown в Telegram HTML. Вход считается недоверенным:
// HTML-теги экранируются, поддерживается только безопасная разметка.
func Format(markdown string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	inCode := false
	code := make([]string, 0, 8)

	flushCode := func() {
		if len(code) == 0 {
			out = append(out, "<pre></pre>")
			return
		}
		out = append(out, "<pre>"+escapeHTML(strings.Join(code, "\n"))+"</pre>")
		code = code[:0]
	}

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
				inCode = false
			} else {
				inCode = true
				code = code[:0]
			}
			continue
		}
		if inCode {
			code = append(code, line)
			continue
		}

		if isTableRow(line) && index+1 < len(lines) && isTableSeparator(lines[index+1]) {
			block := []string{line, lines[index+1]}
			next := index + 2
			for next < len(lines) && isTableRow(lines[next]) {
				block = append(block, lines[next])
				next++
			}
			out = append(out, formatTable(block)...)
			index = next - 1
			continue
		}

		out = append(out, formatLine(line))
	}
	if inCode {
		flushCode()
	}

	result := strings.Join(out, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

func formatLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if horizontalRule.MatchString(trimmed) {
		return ""
	}
	if match := headingPattern.FindStringSubmatch(trimmed); match != nil {
		return "<b>" + formatInline(match[1]) + "</b>"
	}
	if match := listPattern.FindStringSubmatch(line); match != nil {
		return "• " + formatInline(match[1])
	}
	if match := orderedPattern.FindStringSubmatch(line); match != nil {
		return match[1] + ". " + formatInline(match[2])
	}
	return formatInline(trimmed)
}

func formatInline(raw string) string {
	escaped := escapeHTML(raw)
	// inline code — сохраняем содержимое как код, не разбирая markdown внутри
	codeValues := make([]string, 0, 2)
	escaped = codeSpanPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		value := strings.TrimSuffix(strings.TrimPrefix(match, "`"), "`")
		codeValues = append(codeValues, value)
		return "\x00CODE" + itoa(len(codeValues)-1) + "\x00"
	})
	escaped = linkPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		if len(parts) != 3 || !isSafeURL(parts[2]) {
			return match
		}
		href := strings.ReplaceAll(parts[2], `"`, "%22")
		return `<a href="` + href + `">` + parts[1] + `</a>`
	})
	escaped = boldPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		parts := boldPattern.FindStringSubmatch(match)
		value := parts[1]
		if value == "" {
			value = parts[2]
		}
		return "<b>" + value + "</b>"
	})
	for index, value := range codeValues {
		escaped = strings.ReplaceAll(escaped, "\x00CODE"+itoa(index)+"\x00", "<code>"+value+"</code>")
	}
	return escaped
}

func escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func isSafeURL(url string) bool {
	lower := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isTableRow(line string) bool {
	return strings.Count(strings.TrimSpace(line), "|") >= 2
}

func isTableSeparator(line string) bool {
	cells := tableCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !tableSepPattern.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	parts := strings.Split(line, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func formatTable(block []string) []string {
	if len(block) < 2 {
		return nil
	}
	header := tableCells(block[0])
	rows := block[2:]
	out := make([]string, 0, len(rows)+1)
	if len(header) > 0 {
		headerCells := make([]string, 0, len(header))
		for _, cell := range header {
			headerCells = append(headerCells, formatInline(cell))
		}
		out = append(out, "<i>"+strings.Join(headerCells, " · ")+"</i>")
	}
	for _, row := range rows {
		cells := tableCells(row)
		if len(cells) == 0 {
			continue
		}
		line := "• <b>" + formatInline(cells[0]) + "</b>"
		if len(cells) > 1 {
			rest := make([]string, 0, len(cells)-1)
			for _, cell := range cells[1:] {
				rest = append(rest, formatInline(cell))
			}
			line += " — " + strings.Join(rest, " · ")
		}
		out = append(out, line)
	}
	return out
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [8]byte
	pos := len(digits)
	for value > 0 {
		pos--
		digits[pos] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[pos:])
}
