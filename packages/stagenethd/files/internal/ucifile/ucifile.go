// Package ucifile parses and emits OpenWrt UCI configuration files
// (/etc/config/*), preserving section and option ordering.
package ucifile

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Option is a single UCI option or list item.
type Option struct {
	Name  string
	Value string
	List  bool
}

// Section is a UCI section. Anonymous sections have an empty Name.
type Section struct {
	Type    string
	Name    string
	Options []Option
}

// File is an ordered list of UCI sections.
type File struct {
	Sections []Section
}

// splitArgs splits a line on whitespace honoring single and double quotes.
func splitArgs(s string) []string {
	var out []string
	var b strings.Builder
	var quote byte
	inTok := false
	flush := func() {
		if inTok {
			out = append(out, b.String())
			b.Reset()
			inTok = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				b.WriteByte(c)
			}
		case c == '\'' || c == '"':
			inTok = true
			quote = c
		case c == ' ' || c == '\t':
			flush()
		default:
			inTok = true
			b.WriteByte(c)
		}
	}
	flush()
	return out
}

// Parse reads UCI file contents into a File.
func Parse(data string) *File {
	f := &File{}
	var cur *Section
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := splitArgs(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "config":
			if len(fields) < 2 {
				continue
			}
			s := Section{Type: fields[1]}
			if len(fields) >= 3 {
				s.Name = fields[2]
			}
			f.Sections = append(f.Sections, s)
			cur = &f.Sections[len(f.Sections)-1]
		case "option":
			if cur != nil && len(fields) >= 3 {
				cur.Options = append(cur.Options, Option{Name: fields[1], Value: fields[2]})
			}
		case "list":
			if cur != nil && len(fields) >= 3 {
				cur.Options = append(cur.Options, Option{Name: fields[1], Value: fields[2], List: true})
			}
		}
	}
	return f
}

// Read parses a UCI config file from disk.
func Read(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(b)), nil
}

// Write emits the file to w.
func (f *File) Write(w io.Writer) error {
	_, err := io.WriteString(w, f.Text())
	return err
}

// Get returns the value of the first option with the given name, or "".
func (s *Section) Get(name string) string {
	for _, o := range s.Options {
		if o.Name == name && !o.List {
			return o.Value
		}
	}
	return ""
}

// Set updates or appends a scalar option.
func (s *Section) Set(name, value string) {
	for i := range s.Options {
		if s.Options[i].Name == name && !s.Options[i].List {
			s.Options[i].Value = value
			return
		}
	}
	s.Options = append(s.Options, Option{Name: name, Value: value})
}

// Index returns the position of the first section with the given name, or -1.
func (f *File) Index(name string) int {
	for i := range f.Sections {
		if f.Sections[i].Name == name {
			return i
		}
	}
	return -1
}

// Get returns the first section with the given name, or nil.
func (f *File) Get(name string) *Section {
	if i := f.Index(name); i >= 0 {
		return &f.Sections[i]
	}
	return nil
}

// quote renders a value for emission. UCI values are single-line —
// embedded newlines/carriage returns are replaced so a hostile value
// can never break out of its line and inject directives.
func quote(v string) string {
	v = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, v)
	if strings.Contains(v, "'") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return "'" + v + "'"
}

// Text renders the file back to UCI config syntax.
func (f *File) Text() string {
	var b strings.Builder
	for i := range f.Sections {
		s := &f.Sections[i]
		if s.Name != "" {
			fmt.Fprintf(&b, "config %s %s\n", s.Type, quote(s.Name))
		} else {
			fmt.Fprintf(&b, "config %s\n", s.Type)
		}
		for _, o := range s.Options {
			kw := "option"
			if o.List {
				kw = "list"
			}
			fmt.Fprintf(&b, "\t%s %s %s\n", kw, o.Name, quote(o.Value))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
