package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRemoteList(t *testing.T) {
	input := "origin\thttps://github.com/user/repo (fetch)\n" +
		"origin\thttps://github.com/user/repo (push)\n" +
		"upstream\tgit@github.com:org/repo.git (fetch)\n" +
		"upstream\tgit@github.com:org/repo.git (push)\n"

	remotes := parseRemoteList(input)
	assert.Len(t, remotes, 2)

	assert.Equal(t, "origin", remotes[0].Name)
	assert.Equal(t, "https://github.com/user/repo", remotes[0].FetchURL)
	assert.Equal(t, "https://github.com/user/repo", remotes[0].PushURL)

	assert.Equal(t, "upstream", remotes[1].Name)
	assert.Equal(t, "git@github.com:org/repo.git", remotes[1].FetchURL)
	assert.Equal(t, "git@github.com:org/repo.git", remotes[1].PushURL)
}

func TestParseRemoteList_DifferentURLs(t *testing.T) {
	input := "origin\thttps://github.com/user/repo (fetch)\n" +
		"origin\tgit@github.com:user/repo.git (push)\n"

	remotes := parseRemoteList(input)
	assert.Len(t, remotes, 1)

	assert.Equal(t, "origin", remotes[0].Name)
	assert.Equal(t, "https://github.com/user/repo", remotes[0].FetchURL)
	assert.Equal(t, "git@github.com:user/repo.git", remotes[0].PushURL)
}

func TestParseRemoteList_Single(t *testing.T) {
	input := "origin\thttps://github.com/user/repo (fetch)\n" +
		"origin\thttps://github.com/user/repo (push)\n"

	remotes := parseRemoteList(input)
	assert.Len(t, remotes, 1)
	assert.Equal(t, "origin", remotes[0].Name)
}

func TestParseRemoteList_Empty(t *testing.T) {
	remotes := parseRemoteList("")
	assert.Empty(t, remotes)
}

func TestParseRemoteList_VariousURLFormats(t *testing.T) {
	input := "gh\thttps://github.com/user/repo.git (fetch)\n" +
		"gh\thttps://github.com/user/repo.git (push)\n" +
		"ssh\tssh://git@example.com:22/repo.git (fetch)\n" +
		"ssh\tssh://git@example.com:22/repo.git (push)\n" +
		"local\t/home/user/repos/project (fetch)\n" +
		"local\t/home/user/repos/project (push)\n"

	remotes := parseRemoteList(input)
	assert.Len(t, remotes, 3)

	assert.Equal(t, "gh", remotes[0].Name)
	assert.Equal(t, "https://github.com/user/repo.git", remotes[0].FetchURL)

	assert.Equal(t, "ssh", remotes[1].Name)
	assert.Equal(t, "ssh://git@example.com:22/repo.git", remotes[1].FetchURL)

	assert.Equal(t, "local", remotes[2].Name)
	assert.Equal(t, "/home/user/repos/project", remotes[2].FetchURL)
}
