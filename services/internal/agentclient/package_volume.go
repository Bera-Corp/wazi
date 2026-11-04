package agentclient

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

// packageVolumeOut quiesces the reviewed SQLite application while its whole
// data volume is archived. The fixed helper never receives a host bind mount.
func (b *DockerPackageBoundary) packageVolumeOut(ctx context.Context, p PreparedExecution, output io.Writer) (err error) {
	compose, err := b.render(p.Manifest())
	if err != nil {
		return err
	}
	image, err := b.imageConfig(p)
	if err != nil {
		return err
	}
	if _, err = b.core.compose(ctx, compose, "stop", "--timeout", "30", b.shape.Service); err != nil {
		return err
	}
	defer func() {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		if _, startErr := b.core.compose(recovery, compose, "start", b.shape.Service); startErr != nil {
			err = fmt.Errorf("checkpoint application restart failed; reconciliation required")
		}
	}()
	_, err = b.core.run(ctx, nil, output, "run", "--rm", "--network", "none", "--memory", "134217728", "--cpus", "0.25", "--pids-limit", "64", "--security-opt", "no-new-privileges:true", "--pull", "never", "--user", "0", "--entrypoint", "/bin/tar", "--mount", "type=volume,source="+b.config.Runtime.Project+"_"+b.shape.Volume+",target=/wazi-volume,readonly", image, "-cf", "-", "-C", "/wazi-volume", ".")
	return err
}

func (b *DockerPackageBoundary) validatePackageTar(file *os.File) error {
	reader := tar.NewReader(file)
	var total int64
	entries := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("package volume archive invalid")
		}
		name := path.Clean(header.Name)
		entries++
		total += header.Size
		if entries > 100000 || header.Size < 0 || total > int64(b.shape.Storage)*1024*1024 || strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\:\x00") || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeRegA) {
			return fmt.Errorf("package volume archive escapes reviewed surface")
		}
	}
	if entries == 0 {
		return fmt.Errorf("package volume archive empty")
	}
	_, err := file.Seek(0, io.SeekStart)
	return err
}

func (b *DockerPackageBoundary) packageVolumeIn(ctx context.Context, p PreparedExecution, file *os.File, compose []byte) error {
	if err := b.validatePackageTar(file); err != nil {
		return err
	}
	image, err := b.imageConfig(p)
	if err != nil {
		return err
	}
	if _, err = b.core.compose(ctx, compose, "stop", "--timeout", "30", b.shape.Service); err != nil {
		return err
	}
	_, err = b.core.run(ctx, file, nil, "run", "--rm", "--interactive", "--network", "none", "--memory", "134217728", "--cpus", "0.25", "--pids-limit", "64", "--security-opt", "no-new-privileges:true", "--pull", "never", "--user", "0", "--entrypoint", "/bin/sh", "--mount", "type=volume,source="+b.config.Runtime.Project+"_"+b.shape.Volume+",target=/wazi-volume", image, "-ec", "find /wazi-volume -mindepth 1 -delete; tar -xf - -C /wazi-volume; chown 1000:1000 /wazi-volume")
	return err
}

func (b *DockerPackageBoundary) initializePackageData(ctx context.Context, p PreparedExecution) error {
	image, err := b.imageConfig(p)
	if err != nil {
		return err
	}
	_, err = b.core.run(ctx, nil, nil, "run", "--rm", "--network", "none", "--memory", "134217728", "--cpus", "0.25", "--pull", "never", "--user", "0", "--entrypoint", "/bin/chown", "--mount", "type=volume,source="+b.config.Runtime.Project+"_"+b.shape.Volume+",target=/wazi-volume", image, "1000:1000", "/wazi-volume")
	return err
}
