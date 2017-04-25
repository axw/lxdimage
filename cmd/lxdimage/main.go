package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
		if err := processFile(arg); err != nil {
			log.Fatal(err)
		}
	}
}

func processFile(f string) error {
	data, err := ioutil.ReadFile(f)
	if err != nil {
		return err
	}
	var spec lxdimage.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		log.Fatal(err)
	}
	color.Set(defaultColor)
	b := lxdimage.Builder{
		Log: log.New(colorWriter{os.Stderr}, "["+spec.Alias+"] ", log.LstdFlags),
	}
	if err := b.Build(spec); err != nil {
		log.Fatal(err)
	}
	return nil
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
