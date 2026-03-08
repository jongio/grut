package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStatusV2_Ordinary(t *testing.T) {
	input := `# branch.oid abc123
# branch.head main
# branch.upstream origin/main
# branch.ab +1 -2
1 M. N... 100644 100644 100644 abc123 def456 src/main.go
1 .M N... 100644 100644 100644 abc123 def456 README.md
1 MM N... 100644 100644 100644 abc123 def456 internal/app.go
`
	result, err := parseStatusV2(input)
	require.NoError(t, err)
	require.Len(t, result, 3)

	assert.Equal(t, "src/main.go", result[0].Path)
	assert.Equal(t, StatusModified, result[0].StagedStatus)
	assert.Equal(t, StatusUnmodified, result[0].WorktreeStatus)

	assert.Equal(t, "README.md", result[1].Path)
	assert.Equal(t, StatusUnmodified, result[1].StagedStatus)
	assert.Equal(t, StatusModified, result[1].WorktreeStatus)

	assert.Equal(t, "internal/app.go", result[2].Path)
	assert.Equal(t, StatusModified, result[2].StagedStatus)
	assert.Equal(t, StatusModified, result[2].WorktreeStatus)
}

func TestParseStatusV2_Renamed(t *testing.T) {
	input := "2 R. N... 100644 100644 100644 abc123 def456 R100 new_name.go\told_name.go\n"
	result, err := parseStatusV2(input)
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "new_name.go", result[0].Path)
	assert.Equal(t, "old_name.go", result[0].OrigPath)
	assert.Equal(t, StatusRenamed, result[0].StagedStatus)
	assert.Equal(t, StatusUnmodified, result[0].WorktreeStatus)
}

func TestParseStatusV2_Untracked(t *testing.T) {
	input := "? untracked.txt\n"
	result, err := parseStatusV2(input)
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "untracked.txt", result[0].Path)
	assert.Equal(t, StatusUntracked, result[0].StagedStatus)
	assert.Equal(t, StatusUntracked, result[0].WorktreeStatus)
}

func TestParseStatusV2_Ignored(t *testing.T) {
	input := "! ignored.txt\n"
	result, err := parseStatusV2(input)
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "ignored.txt", result[0].Path)
	assert.Equal(t, StatusIgnored, result[0].StagedStatus)
	assert.Equal(t, StatusIgnored, result[0].WorktreeStatus)
}

func TestParseStatusV2_Unmerged(t *testing.T) {
	input := "u UU N... 100644 100644 100644 100644 abc123 def456 ghi789 conflict.go\n"
	result, err := parseStatusV2(input)
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "conflict.go", result[0].Path)
	assert.Equal(t, StatusConflict, result[0].StagedStatus)
	assert.Equal(t, StatusConflict, result[0].WorktreeStatus)
}

func TestParseStatusV2_Empty(t *testing.T) {
	result, err := parseStatusV2("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestParseStatusV2_Mixed(t *testing.T) {
	input := `# branch.oid abc123
# branch.head main
1 A. N... 000000 100644 100644 0000000 abc1234 new_file.go
? another_untracked.txt
1 D. N... 100644 000000 000000 abc1234 0000000 deleted.go
`
	result, err := parseStatusV2(input)
	require.NoError(t, err)
	require.Len(t, result, 3)

	assert.Equal(t, "new_file.go", result[0].Path)
	assert.Equal(t, StatusAdded, result[0].StagedStatus)

	assert.Equal(t, "another_untracked.txt", result[1].Path)
	assert.Equal(t, StatusUntracked, result[1].StagedStatus)

	assert.Equal(t, "deleted.go", result[2].Path)
	assert.Equal(t, StatusDeleted, result[2].StagedStatus)
}

func TestParseStatusBranch(t *testing.T) {
	input := `# branch.oid abc123def456
# branch.head main
# branch.upstream origin/main
# branch.ab +3 -1
1 M. N... 100644 100644 100644 abc123 def456 file.go
`
	sb := ParseStatusBranch(input)

	assert.Equal(t, "abc123def456", sb.OID)
	assert.Equal(t, "main", sb.Head)
	assert.Equal(t, "origin/main", sb.Upstream)
	assert.Equal(t, 3, sb.Ahead)
	assert.Equal(t, 1, sb.Behind)
}

func TestParseStatusBranch_NoBranch(t *testing.T) {
	input := "# branch.oid abc123\n# branch.head (detached)\n"
	sb := ParseStatusBranch(input)
	assert.Equal(t, "(detached)", sb.Head)
	assert.Equal(t, "", sb.Upstream)
	assert.Equal(t, 0, sb.Ahead)
	assert.Equal(t, 0, sb.Behind)
}

func TestStatusCode_String(t *testing.T) {
	assert.Equal(t, "M", StatusModified.String())
	assert.Equal(t, "A", StatusAdded.String())
	assert.Equal(t, "?", StatusUntracked.String())
	assert.Equal(t, " ", StatusUnmodified.String())
}
