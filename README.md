lxdimage builds LXD images by taking an existing image and running shell commands defined within a YAML file.

```
go get github.com/axw/lxdimage/...
lxdimage <image.yml>
```

The format of the YAML image specification is:

```
base: <source-image-name>
alias: <output-image-alias>
commands:
  - command 1
  - command 2
  - etc.
```

For an example, see [examples/opensuseleap.yml](https://raw.githubusercontent.com/axw/lxdimage/master/examples/opensuseleap.yml),
which contains a build specification for a [Juju](github.com/juju/juju)-compatible openSUSE Leap image.

lxdimage accepts http/https URLs as input, e.g.

```
lxdimage https://raw.githubusercontent.com/axw/lxdimage/master/examples/opensuseleap.yml
```
