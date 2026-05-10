package ftpclient

import (
	"context"
	"fmt"
	"strings"
)

// FtpPathJoin joins an FTP directory and a child name.
func FtpPathJoin(parent, name string) string {
	p := strings.TrimRight(strings.TrimSpace(parent), "/")
	n := strings.TrimSpace(name)
	n = strings.TrimLeft(n, "/")
	switch {
	case p == "":
		return n
	case n == "":
		return p
	default:
		return p + "/" + n
	}
}

// ExpandScanDirs returns roots unchanged when scanMaxDepth <= 0. Otherwise performs a BFS
// up to scanMaxDepth levels under each root, deduplicating paths.
func ExpandScanDirs(ctx context.Context, client *Client, roots []string, scanMaxDepth int) ([]string, error) {
	if scanMaxDepth < 0 {
		scanMaxDepth = 0
	}
	if scanMaxDepth > 32 {
		scanMaxDepth = 32
	}
	if scanMaxDepth <= 0 {
		out := make([]string, 0, len(roots))
		for _, p := range roots {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, strings.TrimRight(p, "/"))
		}
		return out, nil
	}

	type queueItem struct {
		path  string
		level int
	}
	seen := make(map[string]struct{})
	order := make([]string, 0, len(roots)*4)

	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		key := strings.TrimRight(p, "/")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		order = append(order, key)
	}

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		root = strings.TrimRight(root, "/")
		q := []queueItem{{path: root, level: 0}}
		for len(q) > 0 {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			it := q[0]
			q = q[1:]
			add(it.path)
			if it.level >= scanMaxDepth {
				continue
			}
			subs, err := client.ListSubdirs(ctx, it.path)
			if err != nil {
				return nil, fmt.Errorf("list subdirs of %q: %w", it.path, err)
			}
			for _, name := range subs {
				child := FtpPathJoin(it.path, name)
				q = append(q, queueItem{path: child, level: it.level + 1})
			}
		}
	}
	return order, nil
}
