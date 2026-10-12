# Contributing

Commando is in active development. Open an issue to discuss substantial changes
before starting a pull request.

## Build and Check

Use Go 1.26 or newer and [Opcode](https://github.com/DannyBen/opcode).
The Windows build uses Bash and MinGW-w64's `x86_64-w64-mingw32-windres`.

```sh
op check
op build-windows
op preview
```

The executable is `dist/commando.exe`. The preview uses fixed sample files and
writes `tmp/agent/screenshots/preview.png`. Linux supports development and
previews; file transfers require Windows.

For headless Linux, install EGL, Wayland and xkbcommon development libraries:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
```

Experimental Windows shell integration is available in [tools/shell-probe](tools/shell-probe).

## Releases

`op release-check` validates the GoReleaser configuration. `op release-snapshot`
builds a local Windows ZIP and checksums without publishing.
Version tags such as `v0.1.0` trigger the GitHub release workflow.
