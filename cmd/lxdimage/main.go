package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/fatih/color"
	"gopkg.in/yaml.v2"

	"github.com/axw/lxdimage"
)

var (
	defaultColor = color.Faint
	logColor     = color.Reset
)

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s FILE [FILE...]\n", os.Args[0])
		os.Exit(2)
	}
	for _, arg := range args {
		log.Println("processing:", arg)
		if err := processArg(arg); err != nil {
			log.Fatal(err)
		}
	}
}

func processArg(arg string) error {
	data, err := fetch(arg)
	if err != nil {
		return err
	}
	var spec lxdimage.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return err
	}
	color.Set(defaultColor)
	b := lxdimage.Builder{
		Log: log.New(colorWriter{os.Stderr}, "["+spec.Alias+"] ", log.LstdFlags),
	}
	if err := b.Build(spec); err != nil {
		return err
	}
	return nil
}

func fetch(arg string) ([]byte, error) {
	url, err := url.Parse(arg)
	if err != nil || url.Scheme == "" {
		return ioutil.ReadFile(arg)
	}
	if url.Scheme == "file" {
		return ioutil.ReadFile(url.Path)
	}
	resp, err := http.Get(arg)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

type colorWriter struct {
	io.Writer
}

func (w colorWriter) Write(data []byte) (int, error) {
	color.Unset()
	defer color.Set(defaultColor)
	n, err := w.Writer.Write(data)
	return n, err
}
