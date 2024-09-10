# Fabric CLI

This repository provides an alternative CLI utility for [Fabric](https://github.com/Fabric-Development/fabric) to what comes shipped by default. This project is written in _GO_ to **maximize performance**, additionally this utility provides more features (aside from performance improvements), like, shell completions and _interactive mode (WIP)_

> [!NOTE]
> Please note that this is a completely optional package. Fabric will operate normally without it.

> [!NOTE]
> This CLI utility is designed to work with the [v0.0.2 rewrite](https://github.com/Fabric-Development/fabric/tree/rewrite) of Fabric. even though there might be some backward compatibility, its not recommended to use with older versions.

## Requirements

Below is a list of dependencies needed to build and install the utility:

```
go
meson
ninja
```

_those are build-time only dependencies. produced binaries are completely standalone\*_

Additionally, You will need a Fabric environment already set. otherwise this utility is useless.

## Build and Install

> [!NOTE]
> For Arch users, a AUR package is proivded as `fabric-cli-git`.

To build and install this utility, run the following command after cloning the repository:

```
meson setup --buildtype=release --prefix=/usr build && sudo meson install -C build
```
