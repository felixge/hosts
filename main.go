package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

const (
	hostsPath   = "/etc/hosts"
	hostsMarker = "# do not edit; managed by github.com/felixge/hosts"
	blockIP     = "127.0.0.1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	hl, err := ReadHostLines(hostsPath)
	if err != nil {
		return err
	}

	var block, update bool
	flag.Parse()

	// TODO(fg) implement "add", "edit" and "remove" commands, for now I just
	// do this manually via vim.
	switch cmd := flag.Arg(0); cmd {
	case "":
	case "block":
		block, update = true, true
	case "unblock":
		block, update = false, true
	default:
		return errors.Errorf("unknown cmd: %q", cmd)
	}

	if update {
		hl.SetBlocked(block, flag.Args()[1:])
		if err := hl.Save(hostsPath); err != nil {
			return err
		}
	}

	return hl.Fprint(os.Stdout)
}

func ReadHostLines(path string) (HostLines, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var hl HostLines
	br := bufio.NewReader(file)
	for i := 1; ; i++ {
		line, isPrefix, err := br.ReadLine()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "could not read line")
		} else if isPrefix {
			return nil, errors.Errorf("line too long: %d: %s", i, line)
		}
		hl = append(hl, HostLine(line))
	}

	return hl, nil
}

type HostLines []HostLine

func (hl HostLines) Fprint(w io.Writer) error {
	var (
		blocked   []*ManagedHost
		unblocked []*ManagedHost
	)
	for _, l := range hl {
		if mh := l.ManagedHost(); mh == nil {
			continue
		} else if mh.Blocked {
			blocked = append(blocked, mh)
		} else {
			unblocked = append(unblocked, mh)
		}
	}

	var err error
	for i, section := range []struct {
		Name  string
		Hosts []*ManagedHost
	}{
		{Name: "Unblocked", Hosts: unblocked},
		{Name: "Blocked", Hosts: blocked},
	} {
		if i != 0 {
			fprintf(&err, w, "\n")
		}
		fprintf(&err, w, "%s:\n", section.Name)
		for _, mh := range section.Hosts {
			fprintf(&err, w, "  %s\n", strings.Join(mh.Hosts, ", "))
		}
	}
	return err
}

func (hl HostLines) SetBlocked(blocked bool, filter []string) {
	for i, l := range hl {
		if mh := l.ManagedHost(); mh == nil {
			continue
		} else {
			match := len(filter) == 0
		outer:
			for _, f := range filter {
				for _, h := range mh.Hosts {
					if strings.Contains(h, f) {
						match = true
						break outer
					}
				}
			}

			if match {
				mh.Blocked = blocked
				hl[i] = mh.HostLine()
			}
		}
	}
}

func (hl HostLines) Save(path string) error {
	hf, err := os.OpenFile(path, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer hf.Close()

	for _, l := range hl {
		if _, err := fmt.Fprintf(hf, "%s\n", l); err != nil {
			return err
		}
	}
	return hf.Close()
}

func fprintf(dst *error, w io.Writer, format string, args ...interface{}) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil && *dst == nil {
		*dst = err
	}
}

type HostLine string

var blockRegexp = regexp.MustCompilePOSIX("^(# )?" + regexp.QuoteMeta(blockIP) + " ([^#]+) " + regexp.QuoteMeta(hostsMarker))

func (h HostLine) ManagedHost() *ManagedHost {
	m := blockRegexp.FindStringSubmatch(string(h))
	if len(m) != 3 {
		return nil
	}
	return &ManagedHost{
		Hosts:   strings.Split(m[2], " "),
		Blocked: m[1] != "# ",
	}
}

type ManagedHost struct {
	Hosts   []string
	Blocked bool
}

func (mh *ManagedHost) HostLine() HostLine {
	var hl string
	if !mh.Blocked {
		hl += "# "
	}
	hl += blockIP + " "
	hl += strings.Join(mh.Hosts, " ") + " "
	hl += hostsMarker
	return HostLine(hl)
}
