// Command paperless-catalogue-permissions prints the module's permissions
// (resource:action) as a JSON array for its catalogue entry (spec 035). It
// depends only on the manifest package, so the release job runs it without
// building the UI.
package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/go-tangra/go-tangra-paperless/v4/pkg/paperlessmanifest"
)

func main() { os.Exit(cataloguePermissions(os.Stdout)) }

func cataloguePermissions(w io.Writer) int {
	if err := json.NewEncoder(w).Encode(paperlessmanifest.PermissionRefs()); err != nil {
		return 1
	}
	return 0
}
