package cloud

import "strings"

func mutatingAction(action string) bool {
	return strings.HasPrefix(action, "Update") || strings.HasPrefix(action, "Create") || strings.HasPrefix(action, "Delete")
}
