package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func gitCheckoutImpl(ctx context.Context, repoURL, ref, destDir string) error {
	/*err := runCmds(destDir, [][]string{
		{"git", "init", "--quiet"},
		{"git", "remote", "add", "origin", repoURL},

		{"git", "config", "core.sparseCheckout", "true"},
		{"git", "sparse-checkout", "set", path},
		{"git", "pull", "--quiet", "--depth", "1", "origin", ref},

		//{"git", "fetch", "--quiet", "--tags", "origin"},
		//{"git", "checkout", "--quiet", ref},
	})*/
	r, err := git.PlainCloneContext(ctx, destDir, false, &git.CloneOptions{
		URL: repoURL,
		// TODO: Auth: ...
		RemoteName:   "origin",
		SingleBranch: true,
		Depth:        1,
		NoCheckout:   true,
	})
	if err != nil {
		return err
	}
	tree, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("git worktree: %w", err)
	}
	// TODO: support sparse checkout, see https://github.com/go-git/go-git/issues/90
	opts := git.CheckoutOptions{}
	if ref != "" {
		opts.Branch = plumbing.ReferenceName("refs/tags/" + ref)
	}
	err = tree.Checkout(&opts)
	if err != nil {
		return err
	}
	return nil
}

/*
func runCmds(dir string, cmds [][]string) error {
	for _, c := range cmds {
		err := runCmd(dir, c[0], c[1:]...)
		if err != nil {
			return err
		}
	}
	return nil
}

func runCmd(dir, cmd string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, cmd, args...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	c.Dir = dir
	err := c.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		cmds := append([]string{cmd}, args...)
		return fmt.Errorf("%s: %s", strings.Join(cmds, " "), msg)
	}
	return err
}
*/
