package lxdimage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// Template describes a LXD image template.
type Template struct {
	Properties map[string]string `yaml:"properties,omitempty"`
	Template   string            `yaml:"template"`
	When       []string          `yaml:"when,omitempty"`

	// Path is the path of the file on disk that the template
	// creates.
	Path string `yaml:"-"`

	// Content is the contents of the template file to create
	// in the image metadata.
	Content string `yaml:"-"`
}

// Builder builds LXD images.
type Builder struct {
	BaseImage string
	Alias     string
	Templates []Template
	Commands  []string
	Log       *log.Logger
}

func (b *Builder) validateConfig() error {
	if b.BaseImage == "" {
		return errors.New("BaseImage must be set")
	}
	if b.Alias == "" {
		return errors.New("Alias must be set")
	}
	// TODO(axw) validate templates
	return nil
}

// Build builds a LXD image, importing it into LXD with the configured alias.
func (b *Builder) Build() error {
	if err := b.validateConfig(); err != nil {
		return fmt.Errorf("validating config: %v", err)
	}

	logger := b.Log
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	ctx := context{log: logger}

	// Start a build container.
	containerName, err := newBuildContainerName()
	if err != nil {
		return err
	}
	if err := ctx.lxc("launch", b.BaseImage, containerName); err != nil {
		return err
	}
	var deleted bool
	defer func() {
		if deleted {
			return
		}
		err := ctx.lxc("delete", "--force", containerName)
		if err != nil {
			logger.Println("Deleting build container", err)
		}
	}()
	if err := ctx.waitContainerNetwork(containerName); err != nil {
		return err
	}

	// Update the build container by running commands inside it,
	// and then publish the container as an image.
	if err := ctx.runCommands(containerName, b.Commands); err != nil {
		return err
	}
	if err := ctx.lxc("stop", containerName); err != nil {
		return err
	}
	if err := ctx.lxc("publish", "--alias="+b.Alias, containerName); err != nil {
		return err
	}
	if err := ctx.lxc("delete", containerName); err != nil {
		return err
	}
	deleted = true

	// Export the image and add the cloud-init templates.
	if len(b.Templates) > 0 {
		if err := ctx.updateImageTemplates(b.Alias, b.Templates); err != nil {
			return err
		}
	}
	return nil
}

func newBuildContainerName() (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", err
	}
	containerName := fmt.Sprintf("lxd-image-builder-%x", random[:])
	return containerName, nil
}

type context struct {
	log *log.Logger
}

func (ctx *context) waitContainerNetwork(container string) error {
	ctx.log.Println("Waiting for network connectivity")

	now := time.Now()
	interval := time.Second
	deadline := now.Add(time.Minute)
	for !now.After(deadline) {
		status, err := getContainerStatus(container)
		if err != nil {
			return err
		}
		if status.State.Status == "Running" {
			for name, network := range status.State.Networks {
				if name == "lo" || network.State != "up" || len(network.Addresses) == 0 {
					continue
				}
				for _, addr := range network.Addresses {
					if addr.Scope == "global" && addr.Family == "inet" {
						return nil
					}
				}
			}
		}
		time.Sleep(interval)
		now = now.Add(interval)
	}
	return errors.New("timed out waiting for network connectivity")
}

type containerStatus struct {
	State struct {
		Status   string `json:"status"`
		Networks map[string]struct {
			Addresses []struct {
				Family string `json:"family"`
				Scope  string `json:"scope"`
			} `json:"addresses"`
			State string `json:"state"`
		} `json:"network"`
	} `json:"state"`
}

func getContainerStatus(container string) (*containerStatus, error) {
	var buf bytes.Buffer
	cmd := exec.Command("lxc", "list", "--format=json", container)
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var statuses []containerStatus
	if err := json.Unmarshal(buf.Bytes(), &statuses); err != nil {
		return nil, err
	}
	return &statuses[0], nil
}

func (ctx *context) runCommands(container string, commands []string) error {
	for _, command := range commands {
		if err := ctx.lxc("exec", container, "--", "/bin/sh", "-c", "exec "+command); err != nil {
			return err
		}
	}
	return nil
}

func (ctx *context) updateImageTemplates(alias string, templates []Template) error {
	tmpdir, err := ioutil.TempDir("", "lxd-image-builder")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	if err := ctx.lxc("image", "export", alias, tmpdir); err != nil {
		return err
	}

	// Images can have one of two formats: a single tarball with
	// both rootfs and metadata in it, or separate rootfs and
	// metadata tarballs.
	//
	// We currently assume that the centos/7 image uses a single
	// tarball only.
	f, err := os.Open(tmpdir)
	if err != nil {
		return err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return err
	}
	if len(names) != 1 {
		return fmt.Errorf(
			"expected a single tarball, found %v (%s)",
			len(names), names,
		)
	}

	// Decompress the tarball, so we can update its contents. We do it
	// like this rather than extracting the whole tarball with "tar xf"
	// to avoid having to run as root, since the tarball contains root-
	// owned special files.
	tarballName := names[0]
	fingerprint := tarballName[:strings.IndexRune(tarballName, '.')]
	switch ext := path.Ext(tarballName); ext {
	case ".gz":
		if err := ctx.run("gunzip", filepath.Join(tmpdir, tarballName)); err != nil {
			return err
		}
		tarballName = strings.TrimSuffix(tarballName, ext)
	default:
		return fmt.Errorf("Unhandled compression type in tarball: %s", tarballName)
	}

	// Extract metadata.yaml, and update it with the cloud-init
	// template references. Also write the templates to disk in
	// the temp dir, and then update the tarball.
	var metadataBuf bytes.Buffer
	tarCmd := exec.Command("tar", "xOf", tarballName, "metadata.yaml")
	tarCmd.Stdin = os.Stdin
	tarCmd.Stdout = &metadataBuf
	tarCmd.Stderr = os.Stderr
	tarCmd.Dir = tmpdir
	if err := tarCmd.Run(); err != nil {
		return err
	}
	metadata := make(map[string]interface{})
	if err := yaml.Unmarshal(metadataBuf.Bytes(), &metadata); err != nil {
		return err
	}

	// Update the metadata with the cloud-init template references,
	// writing it and the template to disk in the temp dir, so we
	// can update the tarball.
	allTemplates := metadata["templates"].(map[interface{}]interface{})
	for _, t := range templates {
		allTemplates[t.Path] = t
	}
	metadataOut, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}

	log.Println("Updating metadata/templates in tarball")
	outTarballName := filepath.Join(tmpdir, "output.tar.gz")
	if err := createFinalTarball(
		outTarballName,
		filepath.Join(tmpdir, tarballName),
		metadataOut,
		gzip.DefaultCompression,
		nil,
	); err != nil {
		return err
	}

	// Import the image tarball over the top of the alias, and finally
	// remove the intermediate image.
	if err := ctx.lxc("image", "import", "--alias="+alias, outTarballName); err != nil {
		return err
	}
	if err := ctx.lxc("image", "delete", fingerprint); err != nil {
		return err
	}
	return nil
}

func createFinalTarball(
	outpath, inpath string,
	metadata []byte,
	compressionLevel int,
	templates []Template,
) error {
	fin, err := os.Open(inpath)
	if err != nil {
		return err
	}
	defer fin.Close()

	fout, err := os.Create(outpath)
	if err != nil {
		return err
	}
	defer fout.Close()

	gzout, err := gzip.NewWriterLevel(fout, compressionLevel)
	if err != nil {
		return err
	}

	in := tar.NewReader(fin)
	out := tar.NewWriter(gzout)
	for {
		h, err := in.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if h.Name == "metadata.yaml" {
			// Ignore metadata.yaml, we'll write a new one below.
			continue
		}
		if err := out.WriteHeader(h); err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
	}

	writeFile := func(name string, content []byte) error {
		h := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := out.WriteHeader(h); err != nil {
			return err
		}
		_, err = out.Write(content)
		return err
	}
	if err := writeFile("metadata.yaml", metadata); err != nil {
		return err
	}
	for _, t := range templates {
		if err := writeFile(path.Join("templates", t.Template), []byte(t.Content)); err != nil {
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := gzout.Close(); err != nil {
		return err
	}
	return fout.Close()
}

func (ctx *context) lxc(args ...string) error {
	return ctx.run("lxc", args...)
}

func (ctx *context) run(arg0 string, args ...string) error {
	ctx.log.Println("Running command:", arg0, strings.Join(args, " "))
	cmd := exec.Command(arg0, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
