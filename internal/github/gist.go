package github

import "context"

// CreateGist creates a secret gist from the file at path using the gh CLI and
// returns the gist's URL. The gist is secret (unlisted) rather than public.
// The `--` separator stops a path that begins with a dash from being parsed as
// a flag.
func CreateGist(ctx context.Context, path string) (string, error) {
	return ghExec(ctx, "gist", "create", "--secret", "--", path)
}
