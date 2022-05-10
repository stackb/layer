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
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/urfave/cli/v2"
)

type config struct {
	filename string
	layerIDs []string
	sort     bool
}

func main() {
	app := &cli.App{
		Name:  "layer",
		Usage: "inspect layers of an image",
		Commands: []*cli.Command{
			{
				Name:  "info",
				Usage: "info prints info about the layers of an image",
				Action: func(c *cli.Context) error {
					cfg := &config{
						filename: c.Args().First(),
					}
					if err := info(cfg); err != nil {
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
						filename: c.Args().First(),
						layerIDs: c.Args().Tail(),
						sort:     c.Bool("sort"),
					}

					if err := files(cfg); err != nil {
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

func getImage(filename string) (v1.Image, error) {
	if filename == "" {
		return nil, fmt.Errorf("no filename provided")
	}
	image, err := tarball.ImageFromPath(filename, nil)
	if err != nil {
		return nil, err
	}
	return image, nil
}

// info returns the config file for the given filename.
func info(cfg *config) error {
	image, err := getImage(cfg.filename)
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

func files(cfg *config) error {

	image, err := getImage(cfg.filename)
	if err != nil {
		return err
	}

	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	if len(cfg.layerIDs) == 0 {
		for _, layer := range layers {
			if err := layerFiles(cfg, layer); err != nil {
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
			if err := layerFiles(cfg, layers[n-1]); err != nil {
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
		if err := layerFiles(cfg, layer); err != nil {
			return err
		}
	}

	return nil
}

func layerFiles(cfg *config, layer v1.Layer) error {
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
