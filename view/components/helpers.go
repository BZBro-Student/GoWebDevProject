package components

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	"github.com/a-h/templ"
)

// CleanHTML repeatedly unescapes nested HTML entities until raw markup remains
func CleanHTML(content string) string {
	s := strings.TrimSpace(content)
	for {
		unescaped := html.UnescapeString(s)
		if unescaped == s {
			break
		}
		s = unescaped
	}
	return s
}

// StripHTMLWrapper removes outer html/body tags if net/html or database inserted them
func StripHTMLWrapper(content string) string {
	s := CleanHTML(content)
	lower := strings.ToLower(s)

	if strings.Contains(lower, "<style") || strings.Contains(lower, "<!doctype") {
		return s
	}

	if strings.HasPrefix(lower, "<html><head></head><body>") && strings.HasSuffix(lower, "</body></html>") {
		s = s[len("<html><head></head><body>") : len(s)-len("</body></html>")]
	}
	return strings.TrimSpace(s)
}

// WrapDocument embeds HTML into a standalone shell with isolated CSS rules
func WrapDocument(content string) string {
	cleaned := StripHTMLWrapper(content)
	if cleaned == "" {
		return ""
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            line-height: 1.6;
            color: #1f2937;
            background-color: #ffffff;
            padding: 1rem;
            margin: 0;
            word-break: break-word;
        }
        h1, h2, h3, h4 { margin-top: 1em; margin-bottom: 0.5em; line-height: 1.25; }
        p { margin-top: 0; margin-bottom: 1em; }
        ul, ol { padding-left: 1.5em; margin-bottom: 1em; }
        li { margin-bottom: 0.25em; }
        code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, monospace; background: #f3f4f6; padding: 0.2em 0.4em; border-radius: 4px; }
        img { max-width: 100%%; height: auto; }
    </style>
</head>
<body>
    %s
</body>
</html>`, cleaned)
}

// SafeDataURI generates a base64 Data URI cast to templ.SafeURL to bypass Templ's URL sanitizer
func SafeDataURI(content string) templ.SafeURL {
	wrapped := WrapDocument(content)
	if wrapped == "" {
		return templ.SafeURL("about:blank")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(wrapped))
	return templ.SafeURL(fmt.Sprintf("data:text/html;charset=utf-8;base64,%s", encoded))
}

func RawHTML(content string) string {
	return CleanHTML(content)
}
