# Source-built image

`Dockerfile.artex` builds ARTEX from this tree: the frontend is exported with
`npm run build:static`, the Go binary is built with `-tags embedui`, and the
runtime stage adds the recon toolset. No prebuilt binary is copied in.

```bash
./build-image.sh .
```

Tags: `artex-local:<commit>` and `artex-local:<commit>-<binary hash prefix>`.
The second one names one exact binary and is what a deployment should pin.

Notes:

- Base images are pinned by digest; `ARTEX_GIT_SHA` lands in image labels.
- Playwright is pinned to 1.64.0 and the Chromium cache is taken from the
  upstream image, because the Playwright CDN download is unreliable here.
- The binary is reproducible: rebuilding the Go stage from scratch produces the
  same sha256.
