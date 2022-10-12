package git

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

type gitURL struct {
	Repo string
	Ref  string
	Path string
}

func (u *gitURL) Dir() *gitURL {
	n := *u
	n.Path = path.Dir(n.Path)
	return &n
}

func (u *gitURL) JoinPath(p ...string) *gitURL {
	n := *u
	n.Path = path.Join(append([]string{n.Path}, p...)...)
	return &n
}

func (u *gitURL) String() string {
	ref := ""
	if u.Ref != "" {
		ref = fmt.Sprintf("?ref=%s", u.Ref)
	}
	path := u.Path
	if path != "" {
		path = fmt.Sprintf("@%s", path)
	}
	return fmt.Sprintf("%s%s%s", u.Repo, path, ref)
}

func parseURL(s string) (*gitURL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(u.Scheme, "git+") {
		return nil, fmt.Errorf("unsupported git url scheme %q", u.Scheme)
	}
	s = s[4:]
	queryStartPos := strings.LastIndex(s, "?")
	if queryStartPos < 0 {
		queryStartPos = len(s)
	}
	repoEndPosition := queryStartPos
	pathStartPos := strings.LastIndex(s, "@")
	if pathStartPos < 0 {
		pathStartPos = queryStartPos
	} else {
		repoEndPosition = pathStartPos
		pathStartPos++
	}
	return &gitURL{
		Repo: s[:repoEndPosition],
		Ref:  u.Query().Get("ref"),
		Path: s[pathStartPos:queryStartPos],
	}, nil
}
