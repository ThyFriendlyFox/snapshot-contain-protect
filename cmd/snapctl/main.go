// Command snapctl is the command line client for snapshotd. It speaks the
// same HTTP API an agent speaks, so anything the tool can do, an agent can do.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const usage = `snapctl controls the local snapshot daemon.

Usage:
  snapctl workset <name> <path> [path...]   declare the paths a workset covers
  snapctl worksets                          list declared worksets
  snapctl snapshot <workset> [label]        checkpoint a workset
  snapctl list <workset>                    print the snapshot graph
  snapctl diff <from-id> <to-id>            print the paths that changed
  snapctl restore <id>                      return the workset to a snapshot
  snapctl prune <id> [--cascade]            delete a snapshot

Flags:
  -addr    daemon address (default 127.0.0.1:7099, or SNAPSHOT_ADDR)
  -json    print the raw response instead of a table
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "snapctl: "+err.Error())
		os.Exit(1)
	}
}

type client struct {
	addr string
	http *http.Client
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("snapctl", flag.ContinueOnError)
	fs.SetOutput(out)
	addr := fs.String("addr", envOr("SNAPSHOT_ADDR", "127.0.0.1:7099"), "daemon address")
	raw := fs.Bool("json", false, "print the raw response")
	fs.Usage = func() { fmt.Fprint(out, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(out, usage)
		return errors.New("no command")
	}

	c := &client{addr: *addr, http: &http.Client{Timeout: 5 * time.Minute}}
	switch rest[0] {
	case "workset":
		return c.workset(out, rest[1:], *raw)
	case "worksets":
		return c.worksets(out, *raw)
	case "snapshot":
		return c.snapshot(out, rest[1:], *raw)
	case "list":
		return c.list(out, rest[1:], *raw)
	case "diff":
		return c.diff(out, rest[1:], *raw)
	case "restore":
		return c.restore(out, rest[1:], *raw)
	case "prune":
		return c.prune(out, rest[1:], *raw)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q; run snapctl help", rest[0])
	}
}

func (c *client) workset(out io.Writer, args []string, raw bool) error {
	if len(args) < 2 {
		return errors.New("usage: snapctl workset <name> <path> [path...]")
	}
	paths := make([]string, 0, len(args)-1)
	for _, p := range args[1:] {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		paths = append(paths, abs)
	}
	body, err := c.call(http.MethodPost, "/worksets", map[string]any{"name": args[0], "paths": paths})
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var w struct {
		Name  string   `json:"name"`
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s covers %s\n", w.Name, strings.Join(w.Paths, " "))
	return nil
}

func (c *client) worksets(out io.Writer, raw bool) error {
	body, err := c.call(http.MethodGet, "/worksets", nil)
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var list []struct {
		Name      string   `json:"name"`
		Paths     []string `json:"paths"`
		Container bool     `json:"container"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return err
	}
	for _, w := range list {
		line := fmt.Sprintf("%-16s %s", w.Name, strings.Join(w.Paths, " "))
		if w.Container {
			line += "  [container]"
		}
		fmt.Fprintln(out, line)
	}
	return nil
}

func (c *client) snapshot(out io.Writer, args []string, raw bool) error {
	if len(args) < 1 {
		return errors.New("usage: snapctl snapshot <workset> [label]")
	}
	req := map[string]any{"workset": args[0], "label": strings.Join(args[1:], " ")}
	body, err := c.call(http.MethodPost, "/snapshot", req)
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var got struct {
		ID       string `json:"id"`
		Duration int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s in %d ms\n", got.ID, got.Duration)
	return nil
}

type node struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parent_id"`
	Label     string  `json:"label"`
	CreatedAt int64   `json:"created_at"`
	Auto      bool    `json:"auto"`
}

func (c *client) list(out io.Writer, args []string, raw bool) error {
	if len(args) != 1 {
		return errors.New("usage: snapctl list <workset>")
	}
	body, err := c.call(http.MethodGet, "/snapshots?workset="+args[0], nil)
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var list []node
	if err := json.Unmarshal(body, &list); err != nil {
		return err
	}
	printTree(out, list)
	return nil
}

// printTree draws the graph the daemon returns flat. Children sit under their
// parent, so a branch created by a restore is visible.
func printTree(out io.Writer, list []node) {
	children := map[string][]node{}
	for _, n := range list {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	for _, kids := range children {
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
	}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, n := range children[parent] {
			kind := "manual"
			if n.Auto {
				kind = "auto"
			}
			fmt.Fprintf(out, "%s%s  %s  %-6s  %s\n",
				strings.Repeat("  ", depth), n.ID,
				time.Unix(n.CreatedAt, 0).Format("2006-01-02 15:04:05"),
				kind, n.Label)
			walk(n.ID, depth+1)
		}
	}
	walk("", 0)
}

func (c *client) diff(out io.Writer, args []string, raw bool) error {
	if len(args) != 2 {
		return errors.New("usage: snapctl diff <from-id> <to-id>")
	}
	body, err := c.call(http.MethodGet, "/diff?from="+args[0]+"&to="+args[1], nil)
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var change struct {
		Added     []string `json:"added"`
		Modified  []string `json:"modified"`
		Deleted   []string `json:"deleted"`
		Truncated bool     `json:"truncated"`
	}
	if err := json.Unmarshal(body, &change); err != nil {
		return err
	}
	for _, p := range change.Added {
		fmt.Fprintln(out, "A "+p)
	}
	for _, p := range change.Modified {
		fmt.Fprintln(out, "M "+p)
	}
	for _, p := range change.Deleted {
		fmt.Fprintln(out, "D "+p)
	}
	if change.Truncated {
		fmt.Fprintln(out, "the list is truncated at 500 paths per category")
	}
	return nil
}

func (c *client) restore(out io.Writer, args []string, raw bool) error {
	if len(args) != 1 {
		return errors.New("usage: snapctl restore <id>")
	}
	body, err := c.call(http.MethodPost, "/restore", map[string]any{"id": args[0], "confirm": true})
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var got struct {
		node
		SafetySnapshot *string `json:"safety_snapshot"`
		SafetyWarning  string  `json:"safety_warning"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return err
	}
	fmt.Fprintf(out, "restored %s; new node %s\n", args[0], got.ID)
	if got.SafetySnapshot != nil {
		fmt.Fprintf(out, "the state before this restore is snapshot %s\n", *got.SafetySnapshot)
	}
	if got.SafetyWarning != "" {
		fmt.Fprintln(out, "warning: "+got.SafetyWarning)
	}
	fmt.Fprintln(out, "processes and sockets do not roll back; restart the process tree")
	return nil
}

func (c *client) prune(out io.Writer, args []string, raw bool) error {
	if len(args) == 0 {
		return errors.New("usage: snapctl prune <id> [--cascade]")
	}
	path := "/snapshots/" + args[0]
	for _, a := range args[1:] {
		if a == "--cascade" {
			path += "?cascade=true"
		} else {
			return fmt.Errorf("unknown flag %q", a)
		}
	}
	body, err := c.call(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if raw {
		return print(out, body)
	}
	var got struct {
		Removed []string `json:"removed"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed %d snapshot(s): %s\n", len(got.Removed), strings.Join(got.Removed, " "))
	return nil
}

// call sends one request and returns the body. A daemon error becomes the
// message the daemon wrote, not a status code the reader must look up.
func (c *client) call(method, path string, payload any) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://"+c.addr+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no daemon at %s: %w", c.addr, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return nil, errors.New(e.Error)
		}
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func print(out io.Writer, body []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		_, err := out.Write(body)
		return err
	}
	_, err := fmt.Fprintln(out, pretty.String())
	return err
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
