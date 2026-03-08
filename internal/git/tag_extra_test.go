package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// parseRemoteTags — pure logic parser, zero-coverage
// ---------------------------------------------------------------------------

func TestParseRemoteTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		localSet map[string]bool
		wantLen  int
		wantTags []string
	}{
		{
			name:     "empty output",
			output:   "",
			localSet: nil,
			wantLen:  0,
		},
		{
			name:     "single remote tag",
			output:   "abc1234567890\trefs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			wantLen:  1,
			wantTags: []string{"v1.0.0"},
		},
		{
			name: "multiple remote tags",
			output: "abc1234567890\trefs/tags/v1.0.0\n" +
				"def4567890abc\trefs/tags/v2.0.0\n",
			localSet: map[string]bool{},
			wantLen:  2,
			wantTags: []string{"v1.0.0", "v2.0.0"},
		},
		{
			name: "skip annotated tag dereferences",
			output: "abc1234567890\trefs/tags/v1.0.0\n" +
				"abc1234567890\trefs/tags/v1.0.0^{}\n",
			localSet: map[string]bool{},
			wantLen:  1,
			wantTags: []string{"v1.0.0"},
		},
		{
			name:     "skip local tags",
			output:   "abc1234567890\trefs/tags/v1.0.0\ndef4567890abc\trefs/tags/v2.0.0\n",
			localSet: map[string]bool{"v1.0.0": true},
			wantLen:  1,
			wantTags: []string{"v2.0.0"},
		},
		{
			name: "skip duplicates",
			output: "abc1234567890\trefs/tags/v1.0.0\n" +
				"abc1234567890\trefs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			wantLen:  1,
		},
		{
			name:     "malformed line",
			output:   "nofieldhere\n",
			localSet: map[string]bool{},
			wantLen:  0,
		},
		{
			name:     "hash truncation",
			output:   "abc1234567890deadbeef\trefs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			wantLen:  1,
		},
		{
			name:     "short hash preserved",
			output:   "abc12\trefs/tags/v1.0.0\n",
			localSet: map[string]bool{},
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseRemoteTags(tt.output, tt.localSet)
			assert.Len(t, got, tt.wantLen)
			for i, name := range tt.wantTags {
				if i < len(got) {
					assert.Equal(t, name, got[i].Name)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TagPush / TagPushAll — validation only (no remote)
// ---------------------------------------------------------------------------

func TestTagPushValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name    string
		remote  string
		tag     string
		wantErr string
	}{
		{name: "empty remote", remote: "", tag: "v1.0.0", wantErr: "tag push remote"},
		{name: "empty tag", remote: "origin", tag: "", wantErr: "tag push name"},
		{name: "invalid remote", remote: "; rm -rf", tag: "v1.0.0", wantErr: "tag push remote"},
		{name: "invalid tag", remote: "origin", tag: "--delete", wantErr: "tag push name"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := client.TagPush(ctx, tt.remote, tt.tag)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestTagPushAllValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	err = client.TagPushAll(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag push all")

	err = client.TagPushAll(ctx, "--evil")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag push all")
}

// ---------------------------------------------------------------------------
// TagListRemote — validation (no remote available)
// ---------------------------------------------------------------------------

func TestTagListRemoteValidation(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	client, err := NewClient(dir)
	assert.NoError(t, err)
	ctx := context.Background()

	_, err = client.TagListRemote(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag list remote")

	_, err = client.TagListRemote(ctx, "--evil")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag list remote")
}
