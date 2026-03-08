package git

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// TagList returns all tags in the repository.
func (c *Client) TagList(ctx context.Context) ([]Tag, error) {
	// Use for-each-ref to get structured tag data.
	format := strings.Join([]string{
		"%(refname:short)",       // tag name
		"%(objectname:short)",    // hash
		"%(objecttype)",          // "commit" for lightweight, "tag" for annotated
		"%(contents:subject)",    // message (first line)
		"%(taggername)",          // tagger name
		"%(creatordate:iso8601)", // date
	}, FieldSep)

	out, err := c.run(ctx, "for-each-ref",
		"--format="+format,
		"--sort=-creatordate",
		"refs/tags/",
	)
	if err != nil {
		return nil, fmt.Errorf("tag list: %w", err)
	}

	return parseTagList(out, FieldSep), nil
}

// parseTagList parses for-each-ref output into Tag entries.
func parseTagList(output, sep string) []Tag { //nolint:unparam // separator kept as parameter for testability
	tags := make([]Tag, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		fields := strings.Split(line, sep)
		if len(fields) < 6 {
			continue
		}

		t := Tag{
			Name:        fields[0],
			Hash:        fields[1],
			IsAnnotated: fields[2] == "tag",
			Message:     fields[3],
			Tagger:      fields[4],
		}

		if d, err := time.Parse("2006-01-02 15:04:05 -0700", fields[5]); err == nil {
			t.Date = d
		}

		tags = append(tags, t)
	}
	return tags
}

// TagCreate creates a new tag. If message is non-empty, creates an annotated tag.
func (c *Client) TagCreate(ctx context.Context, name, ref, message string) error {
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("tag create name: %w", err)
	}

	args := []string{"tag"}
	if message != "" {
		args = append(args, "-a", name, "-m", message)
	} else {
		args = append(args, name)
	}

	if ref != "" {
		if err := ValidateRef(ref); err != nil {
			return fmt.Errorf("tag create ref: %w", err)
		}
		args = append(args, ref)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("tag create: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// TagDelete removes a tag.
func (c *Client) TagDelete(ctx context.Context, name string) error {
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("tag delete: %w", err)
	}

	_, err := c.run(ctx, "tag", "-d", name)
	if err != nil {
		return fmt.Errorf("tag delete: %w", err)
	}
	c.cache.Invalidate()
	return nil
}

// TagListRemote discovers tags on a remote using ls-remote.
// It returns tags that exist on the remote but not locally.
func (c *Client) TagListRemote(ctx context.Context, remote string) ([]Tag, error) {
	if err := ValidateRef(remote); err != nil {
		return nil, fmt.Errorf("tag list remote: %w", err)
	}

	out, err := c.run(ctx, "ls-remote", "--tags", remote)
	if err != nil {
		return nil, fmt.Errorf("tag list remote: %w", err)
	}

	// Build set of local tag names for filtering.
	localTags, localErr := c.TagList(ctx)
	if localErr != nil {
		slog.Debug("TagListRemote: failed to fetch local tags for annotation", "error", localErr)
	}
	localSet := make(map[string]bool, len(localTags))
	for _, t := range localTags {
		localSet[t.Name] = true
	}

	return parseRemoteTags(out, localSet), nil
}

// parseRemoteTags parses "git ls-remote --tags" output and returns tags
// that are not present in localSet.
func parseRemoteTags(output string, localSet map[string]bool) []Tag {
	tags := make([]Tag, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		ref := parts[1]

		// Skip annotated-tag dereference lines (^{}).
		if strings.HasSuffix(ref, "^{}") {
			continue
		}

		name := strings.TrimPrefix(ref, "refs/tags/")
		if localSet[name] || seen[name] {
			continue
		}
		seen[name] = true

		if len(hash) > ShortHashLen {
			hash = hash[:ShortHashLen]
		}
		tags = append(tags, Tag{
			Name: name,
			Hash: hash,
		})
	}
	return tags
}

// TagPush pushes a single tag to a remote.
func (c *Client) TagPush(ctx context.Context, remote, name string) error {
	if err := ValidateRef(remote); err != nil {
		return fmt.Errorf("tag push remote: %w", err)
	}
	if err := ValidateRef(name); err != nil {
		return fmt.Errorf("tag push name: %w", err)
	}

	_, err := c.run(ctx, "push", remote, "refs/tags/"+name)
	if err != nil {
		return fmt.Errorf("tag push: %w", err)
	}
	return nil
}

// TagPushAll pushes all tags to a remote.
func (c *Client) TagPushAll(ctx context.Context, remote string) error {
	if err := ValidateRef(remote); err != nil {
		return fmt.Errorf("tag push all: %w", err)
	}

	_, err := c.run(ctx, "push", remote, "--tags")
	if err != nil {
		return fmt.Errorf("tag push all: %w", err)
	}
	return nil
}
