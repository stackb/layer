package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "layer",
		Usage: "inspect layers of an image",
		Commands: []*cli.Command{
			{
				Name:  "inspect",
				Usage: "print info about the layers of an image",
				Action: func(c *cli.Context) error {
					cfg := &config{
						ref: c.Args().First(),
					}
					if err := inspect(cfg); err != nil {
						return cli.Exit(c.Command.Name+": "+err.Error(), 1)
					}
					return nil
				},
			},
			{
				Name:  "ls",
				Usage: "ls prints the files of a layer",
				Action: func(c *cli.Context) error {
					cfg := &config{
						ref:      c.Args().First(),
						layerIDs: c.Args().Tail(),
						sort:     c.Bool("sort"),
					}

					if err := ls(cfg); err != nil {
						return cli.Exit(c.Command.Name+": "+err.Error(), 1)
					}
					return nil
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "sort",
						Aliases: []string{"S"},
						Usage:   "Sort by size (largest file first) before sorting the operands in lexicographical order.",
					},
				},
			},
		},
	}

	app.Run(os.Args)
}

// config is the configuration for the layer commands.
type config struct {
	// ref is the image ref to inspect, or a path to a tarball.
	ref string
	// layerIDs is the list of layer IDs to inspect.
	layerIDs []string
	// sort is true if the output should be sorted by size.
	sort bool
}

// makeOptions returns the options for crane.
func makeOptions(opts ...crane.Option) crane.Options {
	opt := crane.Options{
		Remote: []remote.Option{
			remote.WithAuthFromKeychain(authn.DefaultKeychain),
		},
	}
	for _, o := range opts {
		o(&opt)
	}
	return opt
}

// getImage returns the image for the given ref.
func getImage(r string) (v1.Image, error) {
	if r == "" {
		return nil, fmt.Errorf("no image ref provided")
	}

	image, err := tarball.ImageFromPath(r, nil)
	if err == nil {
		return image, nil
	}

	image, _, err = getDaemonImage(r)
	if err == nil {
		return image, nil
	}

	image, _, err = getRemoteImage(r)
	if err == nil {
		return image, nil
	}

	return nil, fmt.Errorf("unable to find image %q", r)
}

// getRemoteImage returns the image for the given ref.
func getRemoteImage(r string, opt ...crane.Option) (v1.Image, name.Reference, error) {
	o := makeOptions(opt...)
	ref, err := name.ParseReference(r, o.Name...)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing reference %q: %w", r, err)
	}
	img, err := remote.Image(ref, o.Remote...)
	if err != nil {
		return nil, nil, fmt.Errorf("reading image %q: %w", ref, err)
	}
	return img, ref, nil
}

// getDaemonImage returns the image for the given ref.
func getDaemonImage(r string, opt ...crane.Option) (v1.Image, name.Reference, error) {
	o := makeOptions(opt...)
	ref, err := name.ParseReference(r, o.Name...)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing reference %q: %w", r, err)
	}
	img, err := daemon.Image(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("reading image %q: %w", ref, err)
	}
	return img, ref, nil
}

// inspect prints info about layers.
func inspect(cfg *config) error {
	image, err := getImage(cfg.ref)
	if err != nil {
		return err
	}

	layers, err := image.Layers()
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	tw.Write([]byte("N\tLayer\tSize\n"))
	for i, layer := range layers {
		hash, err := layer.DiffID()
		if err != nil {
			return err
		}
		size, err := layer.Size()
		if err != nil {
			return err
		}
		hs := humanize.Bytes(uint64(size))
		fmt.Fprintf(tw, "%d\t%s\t%s\n", i+1, hash, hs)
	}

	return tw.Flush()
}

// ls prints layers.
func ls(cfg *config) error {
	image, err := getImage(cfg.ref)
	if err != nil {
		return err
	}

	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	if len(cfg.layerIDs) == 0 {
		for _, layer := range layers {
			if err := files(cfg, layer); err != nil {
				return err
			}
		}
		return nil
	}

	for _, id := range cfg.layerIDs {
		if n, err := strconv.Atoi(id); err == nil {
			if n < 1 || n > len(layers) {
				return fmt.Errorf("layer %d does not exist", n)
			}
			if err := files(cfg, layers[n-1]); err != nil {
				return err
			}
			continue
		}
		hash, err := v1.NewHash(id)
		if err != nil {
			return fmt.Errorf("invalid layer id %s: %w", id, err)
		}
		layer, err := image.LayerByDigest(hash)
		if layer == nil {
			layer, err = image.LayerByDiffID(hash)
		}
		if err != nil {
			return fmt.Errorf("layer %s not found: %w", id, err)
		}
		if err := files(cfg, layer); err != nil {
			return err
		}
	}

	return nil
}

// files lists the files in the given layer.
func files(cfg *config, layer v1.Layer) error {
	hash, err := layer.DiffID()
	if err != nil {
		return fmt.Errorf("getting layer diffid: %w", err)
	}

	uncompressed, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("getting layer: %w", err)
	}
	defer uncompressed.Close()

	tarReader := tar.NewReader(uncompressed)

	fmt.Printf("\n--- %s ---\n", hash)
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	tw.Write([]byte("Mode\tSize\tName\n"))

	headers := make([]*tar.Header, 0)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		headers = append(headers, header)
	}

	if cfg.sort {
		sort.Slice(headers, func(i, j int) bool {
			if headers[i].Size == headers[j].Size {
				return headers[i].Name < headers[j].Name
			}
			return headers[i].Size > headers[j].Size
		})
	}

	for _, header := range headers {
		mode := os.FileMode(header.Mode)
		hs := humanize.Bytes(uint64(header.Size))
		switch header.Typeflag {
		case tar.TypeDir:
		default:
			fmt.Fprintf(tw, "%s\t%s\t%s\n", mode, hs, header.Name)
		}
	}

	return tw.Flush()
}
