// Package redact masks passwords, tokens and keys in text shown or exported by ars.
package redact

import (
	"fmt"
	"regexp"
	"strings"
)

const Mask = "[REDACTED]"

type rule struct {
	re *regexp.Regexp
}

var builtin = []string{
	`(?i)((?:pass(?:word|wd)?|pwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret)["']?\s*[=:]\s*["']?)[^\s"'&;,]+`,
	`(?i)(--?(?:password|passwd|pass|token|secret|api-key)\s+)[^\s-][^\s]*`,
	`(?i)(authorization:\s*(?:bearer|basic|negotiate|ntlm)\s+)[^\s"']+`,
	`(?i)((?:set-)?cookie:\s*[^=\s;]+=)[^;\s"']+`,
	`(://[^/\s:@]+:)[^@\s/]+(@)`,
	`\b[0-9a-fA-F]{32}:[0-9a-fA-F]{32}\b`,
	`\$2[abxy]\$\d\d\$[./A-Za-z0-9]{53}`,
	`\$(?:1|5|6|y)\$[^\s:$]+\$[./A-Za-z0-9]{8,}`,
	`\$krb5(?:tgs|asrep)\$[^\s]+`,
	`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`,
	`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]*`,
	`-----BEGIN [A-Z ]*PRIVATE KEY-----`,
}

type Redactor struct {
	rules []rule
}

func New(extra []string) (*Redactor, error) {
	r := &Redactor{}
	for _, p := range builtin {
		r.rules = append(r.rules, rule{regexp.MustCompile(p)})
	}
	for _, p := range extra {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("redact_patterns in config.yml: %q is not a valid regular expression (%w)", p, err)
		}
		r.rules = append(r.rules, rule{re})
	}
	return r, nil
}

func (r *Redactor) Line(s string) string {
	if r == nil {
		return s
	}
	for _, ru := range r.rules {
		s = ru.re.ReplaceAllStringFunc(s, func(m string) string {
			sub := ru.re.FindStringSubmatch(m)
			if len(sub) <= 1 {
				return Mask
			}
			kept := strings.Join(sub[1:], "")
			if kept == "" {
				return Mask
			}
			if len(sub) > 2 {
				return sub[1] + Mask + sub[2]
			}
			return sub[1] + Mask
		})
	}
	return s
}

func (r *Redactor) Text(s string) string {
	if r == nil {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = r.Line(l)
	}
	return strings.Join(lines, "\n")
}
