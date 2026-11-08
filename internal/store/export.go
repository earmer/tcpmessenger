package store

import (
	"bufio"
	"io"
	"strings"

	"tcpmessenger/internal/core"
)

const (
	exportTop = "====================================="
	exportSep = "-------------------------------------"
)

// Export writes entries as text in the classic IP Messenger log format:
//
//	=====================================
//	 From: Name (Group/Host/IP)
//	  at Mon Nov 02 10:00:00 2026 (Sealed)
//	-------------------------------------
//	text
//
// Outgoing entries have one " To: " line per recipient.
func Export(entries []core.LogEntry, w io.Writer) error {
	bw := bufio.NewWriter(w)
	for _, e := range entries {
		bw.WriteString(exportTop + "\n")
		if e.Incoming {
			bw.WriteString(" From: " + userLabel(e.From) + "\n")
		} else if len(e.To) == 0 {
			bw.WriteString(" To: \n")
		} else {
			for _, u := range e.To {
				bw.WriteString(" To: " + userLabel(u) + "\n")
			}
		}
		line := "  at " + e.Time.Format("Mon Jan 02 15:04:05 2006")
		var flags []string
		if e.Sealed {
			flags = append(flags, "Sealed")
		}
		if e.Locked {
			flags = append(flags, "Locked")
		}
		if e.Starred {
			flags = append(flags, "Starred")
		}
		if e.Marked {
			flags = append(flags, "Marked")
		}
		if len(flags) > 0 {
			line += " (" + strings.Join(flags, ", ") + ")"
		}
		bw.WriteString(line + "\n")
		if len(e.Attachments) > 0 {
			names := make([]string, len(e.Attachments))
			for i, a := range e.Attachments {
				names[i] = a.Name
			}
			bw.WriteString("  (files) " + strings.Join(names, ", ") + "\n")
		}
		if e.Note != "" {
			bw.WriteString("  (note) " + strings.ReplaceAll(e.Note, "\n", " ") + "\n")
		}
		bw.WriteString(exportSep + "\n")
		text := strings.ReplaceAll(e.Text, "\r\n", "\n")
		bw.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			bw.WriteString("\n")
		}
		bw.WriteString("\n")
	}
	return bw.Flush()
}

// userLabel is "Name (Group/Host/IP)", leaving out empty parts.
func userLabel(u core.User) string {
	name := u.Name
	if name == "" {
		name = u.Logon
	}
	var parts []string
	for _, p := range []string{u.Group, u.Host, hostOf(u.Addr)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return name
	}
	return name + " (" + strings.Join(parts, "/") + ")"
}

func hostOf(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 && !strings.HasSuffix(addr, "]") {
		return strings.Trim(addr[:i], "[]")
	}
	return addr
}
