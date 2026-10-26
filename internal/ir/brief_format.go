package ir

import (
	"fmt"
	"strings"
)

// FormatBrief renders the compact orientation card (no trailing newline).
func FormatBrief(reply BriefReply) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kind=%s", reply.Kind)
	if reply.Java != "" {
		fmt.Fprintf(&b, " java=%s", reply.Java)
	}
	if reply.SpringBoot != "" {
		fmt.Fprintf(&b, " spring_boot=%s", reply.SpringBoot)
	}
	b.WriteByte('\n')
	fmt.Fprintf(&b, "main_files=%d types=%d test_classes=%d\n", reply.MainFiles, reply.Types, reply.TestClasses)
	if len(reply.Profiles) > 0 {
		fmt.Fprintf(&b, "profiles %s\n", strings.Join(reply.Profiles, ","))
	}
	for _, pkg := range reply.Packages {
		name := pkg.Name
		if name == "" {
			name = "(default)"
		}
		fmt.Fprintf(&b, "pkg %s %d\n", name, pkg.Types)
	}
	for _, h := range reply.HTTP {
		fmt.Fprintf(&b, "http %s %s\n", h.Method, h.Route)
	}
	for _, in := range reply.Ingress {
		fmt.Fprintf(&b, "%s %s\n", in.Kind, in.Symbol)
	}
	if len(reply.OutboundHTTP) > 0 {
		fmt.Fprintf(&b, "out_http %s\n", strings.Join(reply.OutboundHTTP, ","))
	}
	if len(reply.OutboundDB) > 0 {
		fmt.Fprintf(&b, "out_db %s\n", strings.Join(reply.OutboundDB, ","))
	}
	return strings.TrimRight(b.String(), "\n")
}
