# symlink-logger

`symlink-logger` helps retire directory symlinks safely.

It discovers symlinks, replaces selected directory symlinks with FUSE
mountpoints, mirrors the original target directory, logs filesystem access as
JSONL, and can restore the original symlink when you are done observing.

This is useful when you reorganize directories and want to know whether old
compatibility paths are still being used.

## Status

Early macOS-focused CLI. The built-in backend uses
[`go-fuse`](https://github.com/hanwen/go-fuse) and requires a FUSE runtime such
as [macFUSE](https://macfuse.github.io/).

## Install

```bash
go install github.com/johnyoonh/symlink-logger/cmd/symlink-logger@latest
```

From a local checkout:

```bash
go build -o ~/.local/bin/symlink-logger ./cmd/symlink-logger
```

## Workflow

Preview candidates:

```bash
symlink-logger scan --root ~/repos
symlink-logger scan --root ~/repos --recursive
symlink-logger plan --root ~/repos --recursive
```

Replace one symlink with a mountpoint directory:

```bash
symlink-logger replace --old ~/repos/old-project
```

Mount one path and start logging:

```bash
symlink-logger mount --old ~/repos/old-project
```

Replace and mount in one foreground command:

```bash
symlink-logger mount --old ~/repos/old-project --replace
```

Dry-run recursive replacement:

```bash
symlink-logger replace-all --root ~/repos --recursive
```

Apply recursive replacement after reviewing the dry run:

```bash
symlink-logger replace-all --root ~/repos --recursive --apply
```

Mount every directory symlink in a tree:

```bash
symlink-logger mount-all --root ~/repos --recursive --replace
```

Unmount and restore:

```bash
symlink-logger unmount --old ~/repos/old-project --restore
symlink-logger unmount-all --registry ~/repos/repo-symlink-candidates.tsv --restore
symlink-logger restore --old ~/repos/old-project
```

## Logs

The default log path is:

```text
~/.local/state/symlink-logger/access.jsonl
```

Override it with:

```bash
SYMLINK_LOGGER_LOG=/tmp/symlink-access.jsonl symlink-logger mount ...
```

Each event includes timestamp, old path, target path, relative path, operation,
and caller IDs when FUSE provides them.

## Safety Notes

- `replace-all` is dry-run by default.
- Bulk apply refuses more than 100 replacements unless `--max-apply` is raised.
- Recursive mode skips symlinks whose targets are not directories.
- `mount-all` runs in the foreground and keeps all mounts alive in one process.
- Always review the plan before replacing symlinks in a large tree.

## Prior Art

[LoggedFS](https://github.com/rflament/loggedfs) is the closest existing
project at the filesystem layer: it is a transparent FUSE filesystem that logs
operations in a backend filesystem.

`symlink-logger` is not claiming that filesystem logging is novel. Its focus is
the symlink-retirement workflow around that idea: discovery, registry support,
safe replacement, grouped mount/unmount, recursive dry runs, and restoration.

Future versions may support an external `loggedfs` backend. The current version
keeps a small built-in Go FUSE backend and does not copy or embed LoggedFS code.
