package port

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Source reads immutable Git objects. No operation checks out or edits the repo.
type Source struct {
	ctx   context.Context
	repo  string
	Pin   Pin
	Blobs map[string]string
	cache map[string][]byte
}

func git(ctx context.Context, repo string, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return b, nil
}

func OpenSource(ctx context.Context, repo string, pin Pin) (*Source, error) {
	if err := pin.Validate(); err != nil {
		return nil, err
	}
	b, err := git(ctx, repo, nil, "rev-parse", "--verify", "refs/tags/"+pin.Tag+"^{commit}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(b)) != pin.Commit {
		return nil, fmt.Errorf("tag %s does not resolve to pinned commit %s", pin.Tag, pin.Commit)
	}
	b, err = git(ctx, repo, nil, "ls-tree", "-rz", "--full-tree", pin.Commit)
	if err != nil {
		return nil, err
	}
	s := &Source{ctx: ctx, repo: repo, Pin: pin, Blobs: map[string]string{}, cache: map[string][]byte{}}
	for _, entry := range bytes.Split(b, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid git tree entry")
		}
		meta := strings.Fields(string(parts[0]))
		if len(meta) == 3 && meta[1] == "blob" && (meta[0] == "100644" || meta[0] == "100755") {
			s.Blobs[string(parts[1])] = meta[2]
		}
	}
	v, err := s.Read("VERSION")
	if err != nil {
		return nil, err
	}
	if strings.SplitN(string(v), "\n", 2)[0] != pin.Tag {
		return nil, fmt.Errorf("VERSION does not match %s", pin.Tag)
	}
	return s, nil
}

func (s *Source) Read(name string) ([]byte, error) {
	if err := s.Load([]string{name}); err != nil {
		return nil, err
	}
	return s.cache[name], nil
}

func (s *Source) Load(names []string) error {
	var pending []string
	seen := map[string]bool{}
	for _, name := range names {
		if !safePath(name) || s.Blobs[name] == "" {
			return fmt.Errorf("source path is not a regular pinned blob: %s", name)
		}
		if _, ok := s.cache[name]; !ok && !seen[name] {
			pending = append(pending, name)
			seen[name] = true
		}
	}
	if len(pending) == 0 {
		return nil
	}
	var input strings.Builder
	for _, name := range pending {
		fmt.Fprintln(&input, s.Blobs[name])
	}
	b, err := git(s.ctx, s.repo, []byte(input.String()), "cat-file", "--batch")
	if err != nil {
		return err
	}
	r := bufio.NewReader(bytes.NewReader(b))
	for _, name := range pending {
		header, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		f := strings.Fields(header)
		if len(f) != 3 || f[0] != s.Blobs[name] || f[1] != "blob" {
			return fmt.Errorf("invalid blob header: %s", header)
		}
		n, err := strconv.Atoi(f[2])
		if err != nil || n < 0 {
			return fmt.Errorf("invalid blob length")
		}
		data := make([]byte, n)
		if _, err = io.ReadFull(r, data); err != nil {
			return err
		}
		if ch, err := r.ReadByte(); err != nil || ch != '\n' {
			return fmt.Errorf("invalid blob terminator")
		}
		s.cache[name] = data
	}
	return nil
}

func (s *Source) PackageFiles(pkg string) []string {
	prefix := "src/" + pkg + "/"
	var files []string
	for name := range s.Blobs {
		if strings.HasPrefix(name, prefix) && !strings.Contains(name[len(prefix):], "/") && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files
}
