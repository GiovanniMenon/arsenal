package search

import (
	"testing"

	"github.com/ynn3m/arsenal/internal/cheats"
)

func mk(title, name, command, desc string, tags ...string) *cheats.Cheat {
	c := &cheats.Cheat{Title: title, Name: name, Command: command, Description: desc}
	for i := 0; i+1 < len(tags); i += 2 {
		c.CommandTags.Set(tags[i], tags[i+1])
	}
	return c
}

var sample = []*cheats.Cheat{
	mk("nmap", "scan specific ports", "nmap -sV -p <ports> <target>", "Scan the given ports", "plateform", "linux", "cat", "RECON"),
	mk("nmap", "scan all ports", "nmap -p- <target>", "", "plateform", "linux"),
	mk("curl", "download a file", "curl -O <url>", "Fetch with curl over http"),
	mk("hydra", "ssh bruteforce", "hydra -l root -P <wordlist> <ip> ssh", "", "plateform", "windows"),
	mk("Arsenal", "Set ip", ">set ip=<ip>", "save the ip"),
}

func names(l []*cheats.Cheat) []string {
	var n []string
	for _, c := range l {
		n = append(n, c.Name)
	}
	return n
}

func eq(t *testing.T, got []*cheats.Cheat, want ...string) {
	t.Helper()
	g := names(got)
	if len(g) != len(want) {
		t.Fatalf("got %v, want %v", g, want)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("got %v, want %v", g, want)
		}
	}
}

func TestEmptyReturnsAllInOrder(t *testing.T) {
	ix := NewIndex(sample)
	eq(t, ix.Filter(""), "scan specific ports", "scan all ports", "download a file", "ssh bruteforce", "Set ip")
	eq(t, ix.Filter("   "), "scan specific ports", "scan all ports", "download a file", "ssh bruteforce", "Set ip")
}

func TestTermsAreAndedSubstringsCaseInsensitive(t *testing.T) {
	ix := NewIndex(sample)
	eq(t, ix.Filter("NMAP"), "scan specific ports", "scan all ports")
	eq(t, ix.Filter("nmap all"), "scan all ports")
	eq(t, ix.Filter("nmap curl"))
}

func TestSearchesTagsCommandTagsCommandAndDescription(t *testing.T) {
	ix := NewIndex(sample)
	eq(t, ix.Filter("windows"), "ssh bruteforce")
	eq(t, ix.Filter("recon"), "scan specific ports")
	eq(t, ix.Filter("-sV"), "scan specific ports")
	eq(t, ix.Filter("over http"), "download a file")
}

func TestExclusion(t *testing.T) {
	ix := NewIndex(sample)
	eq(t, ix.Filter("nmap !all"), "scan specific ports")
	eq(t, ix.Filter("!nmap !hydra !arsenal"), "download a file")
	eq(t, ix.Filter("!"))
}

func TestFuzzyFallbackForTypos(t *testing.T) {
	list := []*cheats.Cheat{
		mk("tool", "typo hit", "namp --scan", ""),
		mk("tool", "no hit", "ls", "something else"),
	}
	eq(t, NewIndex(list).Filter("nmp"), "typo hit")

	eq(t, NewIndex(sample).Filter("scn"), "scan specific ports", "scan all ports")
}

func TestExactMatchesComeBeforeFuzzyOnes(t *testing.T) {
	list := []*cheats.Cheat{
		mk("a", "fuzzy first in file", "sxcxa", ""),
		mk("b", "exact", "sca", ""),
	}
	eq(t, NewIndex(list).Filter("sca"), "exact", "fuzzy first in file")
}

func TestShortTermsAreNeverFuzzy(t *testing.T) {
	ix := NewIndex(sample)
	eq(t, ix.Filter("sn"))
}

func TestExcludedTermsAreNeverFuzzy(t *testing.T) {
	ix := NewIndex(sample)
	eq(t, ix.Filter("nmap !nmp"), "scan specific ports", "scan all ports")
}

func TestCommonPrefixCompletion(t *testing.T) {
	ix := NewIndex(sample)
	if got := ix.CommonPrefix("nm"); got != "nmap -" {
		t.Errorf("CommonPrefix(nm) = %q", got)
	}
	if got := ix.CommonPrefix("zzz"); got != "zzz" {
		t.Errorf("no match should return the input, got %q", got)
	}
	if got := ix.CommonPrefix(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestTagTermsMatchTagsOnlyByPrefix(t *testing.T) {
	list := []*cheats.Cheat{
		mk("nmap", "scan", "nmap <target>", "", "cat", "RECON/SCAN", "plateform", "linux"),
		mk("curl", "fetch", "curl <url>", "mentions recon and linux in the text"),
		mk("hydra", "brute", "hydra <ip>", "", "cat", "ATTACK/CONNECT"),
	}
	list[1].Tags = "web, Recon-tools"
	ix := NewIndex(list)

	eq(t, ix.Filter("#cat"), "scan", "brute")
	eq(t, ix.Filter("#CAT/recon"), "scan")
	eq(t, ix.Filter("#recon"), "fetch")
	eq(t, ix.Filter("#linux"))
	eq(t, ix.Filter("#web !#cat"), "fetch")
	eq(t, ix.Filter("!#cat"), "fetch")
	eq(t, ix.Filter("#cat #plateform"), "scan")
	eq(t, ix.Filter("#cat curl"))
	eq(t, ix.Filter("#"), "scan", "fetch", "brute")
	eq(t, ix.Filter("!# nmap"), "scan")
}

func TestHashTermsAreOrdinaryTextInTheNotesSearch(t *testing.T) {
	ix := NewTextIndex([][]string{{"issue #42 is open"}, {"nothing here"}})
	if got := ix.Filter("#42"); len(got) != 1 || got[0] != 0 {
		t.Errorf("Filter(#42) = %v", got)
	}
}
