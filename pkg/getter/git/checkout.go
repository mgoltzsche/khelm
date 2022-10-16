package git

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"helm.sh/helm/v3/pkg/repo"
)

func gitCheckoutImpl(ctx context.Context, repoURL, ref string, repo *repo.Entry, destDir string) error {
	cloneOpts := git.CloneOptions{
		URL:        repoURL,
		RemoteName: "origin",
		NoCheckout: true,
	}
	isCommitRef := isCommitSHA(ref)
	if !isCommitRef { // cannot find commit in other branch when checking out single branch
		cloneOpts.SingleBranch = true
		cloneOpts.Depth = 1
	}
	scheme := strings.SplitN(repoURL, ":", 2)[0]
	switch scheme {
	case "https":
		cloneOpts.Auth = &http.BasicAuth{
			Username: repo.Username,
			Password: repo.Password,
		}
	default:
		if repo.Username != "" || repo.Password != "" {
			log.Printf("WARNING: ignoring auth config for %s since authentication is not supported for url scheme %q", repoURL, scheme)
		}
	}
	r, err := git.PlainCloneContext(ctx, destDir, false, &cloneOpts)
	if err != nil {
		return fmt.Errorf("git clone: %w", err)
	}
	tree, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("git worktree: %w", err)
	}
	// TODO: support sparse checkout, see https://github.com/go-git/go-git/issues/90
	refType := "without ref"
	opts := git.CheckoutOptions{}
	if ref != "" {
		if isCommitRef {
			opts.Hash = plumbing.NewHash(ref)
			refType = fmt.Sprintf("commit %s", ref)
		} else {
			opts.Branch = plumbing.ReferenceName(fmt.Sprintf("refs/tags/%s", ref))
			refType = fmt.Sprintf("tag %s", ref)
		}
	}
	err = tree.Checkout(&opts)
	if err != nil {
		return fmt.Errorf("git checkout %s: %w", refType, err)
	}
	/*err := runCmds(destDir, [][]string{
		{"git", "init", "--quiet"},
		{"git", "remote", "add", "origin", repoURL},

		{"git", "config", "core.sparseCheckout", "true"},
		{"git", "sparse-checkout", "set", path},
		{"git", "pull", "--quiet", "--depth", "1", "origin", ref},

		//{"git", "fetch", "--quiet", "--tags", "origin"},
		//{"git", "checkout", "--quiet", ref},
	})*/
	return nil
}

func isCommitSHA(s string) bool {
	if len(s) == 40 {
		if _, err := hex.DecodeString(s); err == nil {
			return true
		}
	}
	return false
}
