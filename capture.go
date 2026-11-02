package main

import "my-app/internal/core"

// openCapture opens the screen capture editor. done receives the edited image,
// or is not called when the user cancels. Stub: replaced by its owner.
func openCapture(a *tcpApp, done func(img core.Attachment)) {}
