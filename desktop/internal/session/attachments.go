package session

import (
	"regexp"
)

var attachmentReference = regexp.MustCompile(`^ho_[a-f0-9]{32}$`)

func validAttachmentRefs(refs []string) bool {
	if len(refs) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !attachmentReference.MatchString(ref) || seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return true
}
func cloneAttachmentRefs(refs []string) []string { return append([]string{}, refs...) }
func equalAttachmentRefs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
