package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type containerIndex map[[2]string]containerInfo

func loadContainerIndex(ctx context.Context, cli *client.Client) (containerIndex, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	idx := make(containerIndex)
	for _, c := range containers {
		project := c.Labels["com.docker.compose.project"]
		svc := c.Labels["com.docker.compose.service"]
		if project == "" || svc == "" {
			continue
		}
		var portParts []string
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				portParts = append(portParts, fmt.Sprintf("%d->%d", p.PublicPort, p.PrivatePort))
			}
		}
		portsStr := strings.Join(portParts, ", ")
		if portsStr == "" {
			portsStr = "-"
		}
		img := c.Image
		if strings.HasPrefix(img, "sha256:") {
			img = img[:19]
		}
		cid := c.ID
		if len(cid) > 12 {
			cid = cid[:12]
		}
		info := containerInfo{ID: cid, State: c.State, Created: c.Created, Image: img, Ports: portsStr}
		key := [2]string{project, svc}
		if existing, ok := idx[key]; !ok || existing.State != "running" {
			idx[key] = info
		}
	}
	return idx, nil
}

func (idx containerIndex) lookup(candidates []string, svc string) (containerInfo, bool) {
	for _, p := range candidates {
		if info, ok := idx[[2]string{p, svc}]; ok {
			return info, true
		}
	}
	return containerInfo{}, false
}

func projectCandidates(folderName, composeName string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		n := normaliseProject(s)
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(folderName)
	if composeName != "" {
		add(composeName)
	}
	return out
}

func normaliseProject(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func formatUptime(created int64, state string) string {
	if created == 0 || state != "running" {
		return "-"
	}
	dur := time.Since(time.Unix(created, 0))
	d := int(dur.Hours()) / 24
	h := int(dur.Hours()) % 24
	m := int(dur.Minutes()) % 60
	s := int(dur.Seconds()) % 60
	return fmt.Sprintf("%s%s:%s%s:%s%s:%s%s",
		cyan("%d", d), dim("d"),
		cyan("%02d", h), dim("h"),
		cyan("%02d", m), dim("m"),
		cyan("%02d", s), dim("s"))
}
