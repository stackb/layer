# layer

Print info about container images.

## Installation

```sh
go install github.com/stackb/layer@latest
```

## Usage

Show layers in an image (tarball filename):

```sh
$ layer info image.tar
```

Show layers in an image (ref):

```
$ layer info index.docker.io/nginx:latest
N  Layer                                                                    Size
1  sha256:9c1b6dd6c1e6be9fdd2b1987783824670d3b0dd7ae8ad6f57dc3cea5739ac71e  31 MB
2  sha256:4b7fffa0f0a4a72b2f901c584c1d4ffb67cce7f033cc7969ee7713995c4d2610  25 MB
3  sha256:f5ab86d69014270bcf4d5ce819b9f5c882b35527924ffdd11fecf0fc0dde81a4  604 B
4  sha256:c876aa251c80272eb01eec011d50650e1b8af494149696b80a606bbeccf03d68  893 B
5  sha256:7046505147d7f3edbf7c50c02e697d5450a2eebe5119b62b7362b10662899d85  667 B
6  sha256:b6812e8d56d65d296e21a639b786e7e793e8b969bd2b109fd172646ce5ebe951  1.4 kB
```

List files in an image:

```sh
layer ls image.tar    # all layers
layer ls -S image.tar # sorted
layer ls image.tar 1  # first layer
layer ls image.tar sha256:8fdc131ec4308d2b9196a38855550dc347e83cc0f47d739754ddeb6e03ac2cbe # by diff ID
```
