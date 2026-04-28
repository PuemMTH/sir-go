package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func findComposeDirs(root string, cfg ScanConfig) []composeDir {
	var result []composeDir
	walkDir(root, root, 0, cfg.Depth, &result)
	return result
}

func walkDir(root, current string, depth, maxDepth int, result *[]composeDir) {
	if depth > maxDepth {
		return
	}
	entries, _ := os.ReadDir(current)
	var files []string
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(current, e.Name()))
		} else if strings.HasPrefix(e.Name(), "docker-compose") {
			files = append(files, filepath.Join(current, e.Name()))
		}
	}
	if len(files) > 0 {
		rel, _ := filepath.Rel(root, current)
		if rel == "." {
			rel = filepath.Base(root)
		}
		*result = append(*result, composeDir{dir: current, relFolder: rel, composeFiles: files})
	}
	for _, d := range dirs {
		walkDir(root, d, depth+1, maxDepth, result)
	}
}

func collectRows(ctx context.Context, cli *client.Client, targetPath string, cfg ScanConfig) scanResult {
	idx, _ := loadContainerIndex(ctx, cli)
	if idx == nil {
		idx = make(containerIndex)
	}

	var res scanResult
	num := 0

	for _, cd := range findComposeDirs(targetPath, cfg) {
		for _, cf := range cd.composeFiles {
			composeName := filepath.Base(cf)
			if cfg.FullPath {
				composeName = cf
			}
			projName, svcs, err := parseCompose(cf)
			if err != nil || len(svcs) == 0 {
				num++
				res.total++
				res.stop++
				res.rows = append(res.rows, Row{Num: num, Folder: cd.relFolder, Compose: composeName, Service: "(parse error)", Uptime: "-", Image: "-", Ports: "-", Status: StatusError})
				continue
			}
			cands := projectCandidates(filepath.Base(cd.dir), projName)
			for _, svc := range svcs {
				num++
				res.total++
				info, found := idx.lookup(cands, svc)
				r := Row{Num: num, Folder: cd.relFolder, Compose: composeName, Service: svc, State: info.State, Uptime: "-", ContainerID: "-", Image: "-", Ports: "-", Status: StatusStopped}
				if found {
					r.Uptime = formatUptime(info.Created, info.State)
					r.ContainerID = info.ID
					r.FullContainerID = info.FullID
					r.Image = info.Image
					r.Ports = info.Ports
					switch info.State {
					case "running":
						r.Status = StatusRunning
						res.run++
					case "":
						res.stop++
					default:
						r.Status = StatusOther
						res.stop++
					}
				} else {
					res.stop++
				}
				res.rows = append(res.rows, r)
			}
		}
	}
	return res
}

// collectAllContainers lists ALL Docker Compose containers directly from the daemon.
// Used when no path argument is provided.
func collectAllContainers(ctx context.Context, cli *client.Client, cfg ScanConfig) scanResult {
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return scanResult{}
	}

	var res scanResult
	num := 0

	for _, c := range containers {
		project := c.Labels["com.docker.compose.project"]
		svc := c.Labels["com.docker.compose.service"]
		if project == "" || svc == "" {
			continue
		}

		num++
		res.total++

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

		composeFilePath := c.Labels["com.docker.compose.project.config_files"]
		if composeFilePath == "" {
			composeFilePath = "-"
		} else {
			if strings.Contains(composeFilePath, ",") {
				composeFilePath = strings.Split(composeFilePath, ",")[0]
			}
			if !cfg.FullPath {
				composeFilePath = filepath.Base(composeFilePath)
			}
		}

		r := Row{
			Num:             num,
			Folder:          project,
			Compose:         composeFilePath,
			Service:         svc,
			State:           c.State,
			Uptime:          formatUptime(c.Created, c.State),
			ContainerID:     cid,
			FullContainerID: c.ID,
			Image:           img,
			Ports:           portsStr,
		}

		switch c.State {
		case "running":
			r.Status = StatusRunning
			res.run++
		case "":
			r.Status = StatusStopped
			res.stop++
		default:
			r.Status = StatusOther
			res.stop++
		}
		res.rows = append(res.rows, r)
	}
	return res
}
