trashdump
=========

pull an oci image from any reg (obsiously) and extract its rootfs to a local dir. 
handles multiplatform manifests, private regs, and insec http regs.

### some usage example
```bash
$ trashdump alpine:edge
$ trashdump -o ./rootfs -p linux/arm64 ghcr.io/some/image:latest
```

## install

```bash
go install github.com/idkmaybdeveloper/trashdump@latest
```

## how does it work?

trashdump uses [google/go-containerregistry](https://github.com/google/go-containerregistry) (crane) under the hood. step by step, it will:

* parse the image reference and resolve it against the registry;
* authenticate using da credentials;
* fetch the image manifest and select platform variant;
* assemble all layers into a single tar via `crane.Export`;
* extract the tar to the output directory, handling regular files, etc.
