package contributors

import (
	"fmt"
	"strings"
	"time"
)

// FormatChangelog returns a ### Contributors section for CHANGELOG.md.
func FormatChangelog(contributors []Contributor) string {
	if len(contributors) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Contributors\n\n")
	b.WriteString("Thanks to the following people for their contributions to this release:\n\n")

	for _, c := range contributors {
		b.WriteString(fmt.Sprintf("- **%s**\n", c.Name))
	}

	// Highlight first-time contributors.
	var firstTimers []string
	for _, c := range contributors {
		if c.IsFirstTime {
			firstTimers = append(firstTimers, fmt.Sprintf("**%s**", c.Name))
		}
	}
	if len(firstTimers) > 0 {
		b.WriteString(fmt.Sprintf("\nNew contributors: %s — welcome! 🎉\n", strings.Join(firstTimers, ", ")))
	}

	return b.String()
}

// FormatReleaseNotes returns a contributor acknowledgment section for
// GitHub Release notes (injected via GoReleaser footer or workflow).
func FormatReleaseNotes(contributors []Contributor) string {
	if len(contributors) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Contributors\n\n")
	b.WriteString("Thank you to everyone who contributed to this release:\n\n")

	for _, c := range contributors {
		b.WriteString(fmt.Sprintf("- **%s**\n", c.Name))
	}

	var firstTimers []string
	for _, c := range contributors {
		if c.IsFirstTime {
			firstTimers = append(firstTimers, fmt.Sprintf("**%s**", c.Name))
		}
	}
	if len(firstTimers) > 0 {
		b.WriteString(fmt.Sprintf("\n✨ New contributors: %s — welcome to grüt!\n", strings.Join(firstTimers, ", ")))
	}

	return b.String()
}

// FormatContributorsMD generates the full CONTRIBUTORS.md content.
func FormatContributorsMD(contributors []Contributor) string {
	var b strings.Builder
	b.WriteString("# Contributors\n\n")
	b.WriteString("Thank you to everyone who has contributed to grüt! This file is\n")
	b.WriteString("auto-generated from git history during each release.\n\n")
	b.WriteString("## All Contributors\n\n")
	b.WriteString("| Name | Contributions |\n")
	b.WriteString("|------|---------------|\n")

	for _, c := range contributors {
		b.WriteString(fmt.Sprintf("| **%s** | %d |\n", c.Name, c.CommitCount))
	}

	b.WriteString(fmt.Sprintf("\n---\n\n*Last updated: %s*\n", time.Now().UTC().Format("2006-01-02")))

	return b.String()
}
