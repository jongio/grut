package git

import (
	"context"
	"fmt"
	"strings"
)

// Push pushes commits to a remote.
func (c *Client) Push(ctx context.Context, opts PushOpts) error {
	args := []string{"push"}

	if opts.Force {
		args = append(args, "--force")
	} else if opts.ForceWith {
		args = append(args, "--force-with-lease")
	}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	if opts.Remote != "" {
		if err := ValidateRef(opts.Remote); err != nil {
			return fmt.Errorf("push remote: %w", err)
		}
		args = append(args, opts.Remote)
	}
	if opts.Branch != "" {
		if err := ValidateRef(opts.Branch); err != nil {
			return fmt.Errorf("push branch: %w", err)
		}
		args = append(args, opts.Branch)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

// Pull fetches and integrates changes from a remote.
func (c *Client) Pull(ctx context.Context, opts PullOpts) error {
	args := []string{"pull"}

	if opts.Rebase {
		args = append(args, "--rebase")
	}
	if opts.NoRebase {
		args = append(args, "--no-rebase")
	}
	if opts.Remote != "" {
		if err := ValidateRef(opts.Remote); err != nil {
			return fmt.Errorf("pull remote: %w", err)
		}
		args = append(args, opts.Remote)
	}
	if opts.Branch != "" {
		if err := ValidateRef(opts.Branch); err != nil {
			return fmt.Errorf("pull branch: %w", err)
		}
		args = append(args, opts.Branch)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// Fetch downloads objects and refs from a remote.
func (c *Client) Fetch(ctx context.Context, opts FetchOpts) error {
	args := []string{"fetch"}

	if opts.Prune {
		args = append(args, "--prune")
	}
	if opts.Tags {
		args = append(args, "--tags")
	}
	if opts.All {
		args = append(args, "--all")
	}
	if opts.Remote != "" {
		if err := ValidateRef(opts.Remote); err != nil {
			return fmt.Errorf("fetch remote: %w", err)
		}
		args = append(args, opts.Remote)
	}
	if opts.Refspec != "" {
		if err := ValidateRefspec(opts.Refspec); err != nil {
			return fmt.Errorf("fetch refspec: %w", err)
		}
		args = append(args, opts.Refspec)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// RemoteList returns all configured remotes with their fetch and push URLs.
func (c *Client) RemoteList(ctx context.Context) ([]Remote, error) {
	out, err := c.run(ctx, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("remote list: %w", err)
	}
	return parseRemoteList(out), nil
}

// parseRemoteList parses "git remote -v" output into Remote entries.
// Each remote has two lines: one for fetch and one for push.
//
//	origin	https://github.com/user/repo (fetch)
//	origin	https://github.com/user/repo (push)
func parseRemoteList(output string) []Remote {
	remotes := make(map[string]*Remote)
	var order []string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		// Split on tab: name<TAB>url (direction)
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		rest := parts[1]

		r, ok := remotes[name]
		if !ok {
			r = &Remote{Name: name}
			remotes[name] = r
			order = append(order, name)
		}

		if strings.HasSuffix(rest, "(fetch)") {
			r.FetchURL = strings.TrimSpace(strings.TrimSuffix(rest, "(fetch)"))
		} else if strings.HasSuffix(rest, "(push)") {
			r.PushURL = strings.TrimSpace(strings.TrimSuffix(rest, "(push)"))
		}
	}

	result := make([]Remote, 0, len(order))
	for _, name := range order {
		result = append(result, *remotes[name])
	}
	return result
}

// RemoteAdd adds a new remote with the given name and URL.
func (c *Client) RemoteAdd(ctx context.Context, name, url string) error {
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("remote add name: %w", err)
	}
	if err := ValidateArg(url); err != nil {
		return fmt.Errorf("remote add url: %w", err)
	}

	_, err := c.run(ctx, "remote", "add", name, url)
	if err != nil {
		return fmt.Errorf("remote add: %w", err)
	}
	return nil
}

// RemoteRemove removes a remote by name.
func (c *Client) RemoteRemove(ctx context.Context, name string) error {
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("remote remove name: %w", err)
	}

	_, err := c.run(ctx, "remote", "remove", name)
	if err != nil {
		return fmt.Errorf("remote remove: %w", err)
	}
	c.cache.Invalidate()
	return nil
}
