package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"sir/internal/types"
)

type composeDir struct {
	dir          string
	relFolder    string
	composeFiles []string
}

func findComposeDirs(root string, cfg types.ScanConfig) []composeDir {
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

func CollectRows(ctx context.Context, cli *client.Client, targetPath string, cfg types.ScanConfig) types.ScanResult {
	idx, _ := loadContainerIndex(ctx, cli)
	if idx == nil {
		idx = make(containerIndex)
	}

	var res types.ScanResult
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
				res.Total++
				res.Stop++
				res.Rows = append(res.Rows, types.Row{Num: num, Folder: cd.relFolder, Compose: composeName, Service: "(parse error)", Uptime: "-", Image: "-", Ports: "-", Status: types.StatusError})
				continue
			}
			cands := projectCandidates(filepath.Base(cd.dir), projName)
			for _, svc := range svcs {
				num++
				res.Total++
				info, found := idx.lookup(cands, svc)
				r := types.Row{Num: num, Folder: cd.relFolder, Compose: composeName, Service: svc, State: info.State, Uptime: "-", ContainerID: "-", Image: "-", Ports: "-", Status: types.StatusStopped}
				if found {
					r.Uptime = formatUptime(info.Created, info.State)
					r.ContainerID = info.ID
					r.FullContainerID = info.FullID
					r.Image = info.Image
					r.Ports = info.Ports
					switch info.State {
					case "running":
						r.Status = types.StatusRunning
						res.Run++
					case "":
						res.Stop++
					default:
						r.Status = types.StatusOther
						res.Stop++
					}
				} else {
					res.Stop++
				}
				res.Rows = append(res.Rows, r)
			}
		}
	}
	return res
}

func CollectAllContainers(ctx context.Context, cli *client.Client, cfg types.ScanConfig) types.ScanResult {
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return types.ScanResult{}
	}

	var res types.ScanResult
	num := 0

	for _, c := range containers {
		project := c.Labels["com.docker.compose.project"]
		svc := c.Labels["com.docker.compose.service"]
		if project == "" || svc == "" {
			continue
		}

		num++
		res.Total++

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

		r := types.Row{
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
			r.Status = types.StatusRunning
			res.Run++
		case "":
			r.Status = types.StatusStopped
			res.Stop++
		default:
			r.Status = types.StatusOther
			res.Stop++
		}
		res.Rows = append(res.Rows, r)
	}
	return res
}
