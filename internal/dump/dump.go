// TODO: move to /pkg/??
package dump

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Options struct {
	OutputDir string
	Platform  string
	Username  string
	Password  string
	Insecure  bool
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func refToDir(ref string) string {
	parts := strings.Split(ref, "/")
	last := parts[len(parts)-1]
	return strings.NewReplacer(":", "_", "@", "_").Replace(last)
}

func parsePlatform(s string) (v1.Platform, error) {
	parts := strings.SplitN(s, "/", 3)
	if len(parts) < 2 {
		return v1.Platform{}, fmt.Errorf("invalid platform %q: want os/arch[/variant]", s)
	}
	p := v1.Platform{OS: parts[0], Architecture: parts[1]}
	if len(parts) == 3 {
		p.Variant = parts[2]
	}
	return p, nil
}

func extractTar(ctx context.Context, r io.Reader, destDir string) error {
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	prefix := absDestDir + string(os.PathSeparator)

	tr := tar.NewReader(r)
	var count int64

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		target := filepath.Join(absDestDir, filepath.FromSlash(hdr.Name))
		if target != absDestDir && !strings.HasPrefix(target, prefix) {
			slog.Warn("skipping path escape", "path", hdr.Name)
			continue
		}

		mode := hdr.FileInfo().Mode().Perm()

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0700); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("mkdir parent: %w", err)
			}
			if err := writeFile(target, tr, mode); err != nil {
				return err
			}

		case tar.TypeSymlink:
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil && !os.IsExist(err) {
				slog.Warn("symlink failed", "path", hdr.Name, "err", err)
			}

		case tar.TypeLink:
			linkSrc := filepath.Join(absDestDir, filepath.FromSlash(hdr.Linkname))
			os.Remove(target)
			if err := os.Link(linkSrc, target); err != nil {
				slog.Warn("hardlink failed", "path", hdr.Name, "err", err)
			}
		}

		count++
		if count%1000 == 0 {
			slog.Info("progress", "files", count)
		}
	}

	slog.Info("done", "files", count)
	return nil
}

func Dump(ctx context.Context, imageRef string, opts Options) error {
	nameOpts := []name.Option{}
	if opts.Insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	ref, err := name.ParseReference(imageRef, nameOpts...)
	if err != nil {
		return fmt.Errorf("parse ref: %w", err)
	}

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}
	if opts.Username != "" {
		remoteOpts = append(remoteOpts, remote.WithAuth(
			authn.FromConfig(authn.AuthConfig{Username: opts.Username, Password: opts.Password}),
		))
	}
	if opts.Platform != "" {
		p, err := parsePlatform(opts.Platform)
		if err != nil {
			return err
		}
		remoteOpts = append(remoteOpts, remote.WithPlatform(p))
	}

	slog.Info("fetching image", "ref", imageRef)
	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return fmt.Errorf("fetch image: %w", err)
	}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = refToDir(imageRef)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	slog.Info("extracting rootfs", "output", outDir)

	/*
	 * crane.Export assembles all layers into a single flat tar, applying
	 * whiteout markers (.wh.* files) along the way - so the result is a
	 * clean view of the final filesystem without any OCI layer cruft.
	 *
	 * we pipe it straight into the tar extractor to avoid buffering the
	 * whole thing in memory.
	 */
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(crane.Export(img, pw))
	}()

	err = extractTar(ctx, pr, outDir)
	pr.CloseWithError(err)
	return err
}
