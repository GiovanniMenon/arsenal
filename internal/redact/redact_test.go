package redact

import (
	"strings"
	"testing"
)

func TestBuiltinPatternsMaskTheUsualSecrets(t *testing.T) {
	r, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ in, want string }{
		{"mysql -u root --password hunter2 -h 10.10.10.5", "mysql -u root --password [REDACTED] -h 10.10.10.5"},
		{"export TOKEN=abc123def", "export TOKEN=[REDACTED]"},
		{"curl -d 'password=s3cret&user=admin' http://example.local", "curl -d 'password=[REDACTED]&user=admin' http://example.local"},
		{`{"api_key": "k-123", "name": "x"}`, `{"api_key": "[REDACTED]", "name": "x"}`},
		{"curl -H 'Authorization: Bearer eyJhbGciOi.aaaaaaaaaa.bbbbbbbbbb' https://example.local", "curl -H 'Authorization: Bearer [REDACTED]' https://example.local"},
		{"curl -H 'Authorization: Basic YWRtaW46YWRtaW4=' https://example.local", "curl -H 'Authorization: Basic [REDACTED]' https://example.local"},
		{"Cookie: session=abcdef123456; theme=dark", "Cookie: session=[REDACTED]; theme=dark"},
		{"git clone https://deploy:Pa55w0rd@example.local/repo.git", "git clone https://deploy:[REDACTED]@example.local/repo.git"},
		{"admin:500:aad3b435b51404eeaad3b435b51404ee:31d6cfe0d16ae931b73c59d7e0c089c0:::", "admin:500:[REDACTED]:::"},
		{"root:$6$saltsalt$abcdefghijklmnopqrstuv:19000:0:99999:7:::", "root:[REDACTED]:19000:0:99999:7:::"},
		{"$2y$10$" + strings.Repeat("a", 53), Mask},
		{"$krb5tgs$23$*svc$EXAMPLE.LOCAL$spn*$aabbcc$ddeeff", Mask},
		{"aws configure set aws_access_key_id AKIAABCDEFGHIJKLMNOP", "aws configure set aws_access_key_id [REDACTED]"},
		{"-----BEGIN OPENSSH PRIVATE KEY-----", Mask},
	}
	for _, c := range cases {
		if got := r.Line(c.in); got != c.want {
			t.Errorf("Line(%q)\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

func TestOrdinaryCommandsAreLeftAlone(t *testing.T) {
	r, _ := New(nil)
	for _, in := range []string{
		"nmap -sV -p 22,80 10.10.10.5",
		"nmap -p- --open 10.10.10.5",
		"ls -la /etc/passwd",
		"cat /etc/shadow",
		"ssh -i key.pem user@example.local",
		"echo passwords are in the vault",
		"hydra -L users.txt -P rockyou.txt ssh://10.10.10.5",
		"curl -k https://example.local/login",
		"password reset page",
	} {
		if got := r.Line(in); got != in {
			t.Errorf("changed an ordinary line:\n in  %q\n out %q", in, got)
		}
	}
}

func TestExtraPatternsAndErrors(t *testing.T) {
	r, err := New([]string{`ACME-\d{6}`, `(client=)\w+`})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Line("ticket ACME-123456 for client=initech"); got != "ticket [REDACTED] for client=[REDACTED]" {
		t.Errorf("got %q", got)
	}
	if _, err := New([]string{"(unclosed"}); err == nil || !strings.Contains(err.Error(), "redact_patterns") {
		t.Errorf("an invalid pattern must be reported: %v", err)
	}
}

func TestTextMasksEveryLineAndANilRedactorChangesNothing(t *testing.T) {
	r, _ := New(nil)
	in := "line one\npassword=abc\nline three"
	if got := r.Text(in); got != "line one\npassword=[REDACTED]\nline three" {
		t.Errorf("got %q", got)
	}
	var none *Redactor
	if none.Line("password=abc") != "password=abc" || none.Text("a\nb") != "a\nb" {
		t.Error("a nil redactor must pass text through")
	}
}
