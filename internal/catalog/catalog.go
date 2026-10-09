// Package catalog loads each app's event catalog: a YAML file that documents modules,
// screens, events and parameters for both people and AI assistants.
//
// The file is shown verbatim by `insight describe`, so free-form comments are welcome.
// Only a few fields are parsed (see docs/event-catalog.md):
//
//	display_name: Pawprint
//	modules:
//	  timeline: Journal timeline
//	events:
//	  - name: entry.created
//	    desc: A journal entry was saved
//
// Events that reach the server but are missing from the catalog are listed by describe.
package catalog

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Set holds the catalogs of all configured apps, keyed by app key.
type Set struct {
	text map[string]string
}

// Load reads the catalog file of each app; apps with an empty path have no catalog.
func Load(paths map[string]string) (*Set, error) {
	s := &Set{text: map[string]string{}}
	for app, p := range paths {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("catalog for %s: %w", app, err)
		}
		s.text[app] = string(b)
	}
	return s, nil
}

// FromText builds a set from in-memory catalogs (tests and the demo).
func FromText(text map[string]string) *Set { return &Set{text: text} }

var eventNameRE = regexp.MustCompile(`(?m)^  - name: ([a-z0-9_.]+)\s*$`)

// Text returns an app's catalog verbatim, or "" when it has none.
func (s *Set) Text(app string) string { return s.text[app] }

// Apps returns the keys of apps that have a catalog, sorted.
func (s *Set) Apps() []string {
	out := make([]string, 0, len(s.text))
	for k := range s.text {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// EventNames returns the event names documented for an app.
func (s *Set) EventNames(app string) map[string]bool {
	out := map[string]bool{}
	for _, m := range eventNameRE.FindAllStringSubmatch(s.Text(app), -1) {
		out[m[1]] = true
	}
	return out
}

var (
	eventDescRE   = regexp.MustCompile(`(?m)^  - name: ([a-z0-9_.]+)\s*\n    desc: (.+)$`)
	displayNameRE = regexp.MustCompile(`(?m)^display_name: (.+)$`)
	moduleLineRE  = regexp.MustCompile(`^  ([a-z0-9_]+): (.+)$`)
)

// EventDescs returns event name → description.
func (s *Set) EventDescs(app string) map[string]string {
	out := map[string]string{}
	for _, m := range eventDescRE.FindAllStringSubmatch(s.Text(app), -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// DisplayName returns the catalog's display_name, or "".
func (s *Set) DisplayName(app string) string {
	if m := displayNameRE.FindStringSubmatch(s.Text(app)); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// Modules returns the modules section as ordered (key, description) pairs.
func (s *Set) Modules(app string) [][2]string {
	var out [][2]string
	in := false
	for _, line := range strings.Split(s.Text(app), "\n") {
		switch {
		case line == "modules:":
			in = true
		case in && moduleLineRE.MatchString(line):
			m := moduleLineRE.FindStringSubmatch(line)
			out = append(out, [2]string{m[1], strings.TrimSpace(m[2])})
		case in && line != "" && !strings.HasPrefix(line, " "):
			return out
		}
	}
	return out
}
