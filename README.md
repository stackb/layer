# layer

Print info about container image tarballs.

## Installation

```sh
go install github.com/stackb/layer@latest
```

## Usage

Show layers in an image:

```sh
layer info myimage.tar
```

List files in an image:

```sh
layer ls myimage.tar    # all layers
layer ls -S myimage.tar # sorted
layer ls myimage.tar 1  # first layer
layer ls myimage.tar sha256:8fdc131ec4308d2b9196a38855550dc347e83cc0f47d739754ddeb6e03ac2cbe # by diff ID
```
