# Redux Multilingual + Adapter

The Dictation page offers **Redux Multilingual + Adapter**, provider **Parakeet**,
between Base Multilingual and Medium Multilingual on systemd desktops. Its
copy explicitly names the Voxtype Redux adapter. The model supports 25 European
languages; the local accuracy benchmarks cover English only.

## Install, select, switch and remove

Clicking the missing option builds a CPU-only parakeet.cpp runtime from pinned
MIT-licensed source and downloads a checksum-pinned Redux GGUF. It installs the
bundled MIT-licensed Python adapter, a user worker service, the model, licensing
notice and a receipt. Python 3.11+, Git, CMake, Make and a C++ compiler are needed
for installation. The Hub never runs sudo or downloads a proprietary runtime.
Model weights are CC-BY-4.0, attributed in the installed notice. Dependency checks
fail before making changes when native build tools are unavailable.

Only a successful installation selects the option. Stock Voxtype's CLI backend
calls the resident adapter; the worker uses two backend slots, four compute
threads per slot and quiet splits near 30 seconds. Recording limit:120 seconds.
Selecting another model stops/disables the worker and removes the Redux service
dependency, retaining the installation for quick reuse. Turning dictation off
stops both services.

The minus button removes the model, adapter, worker unit and privately built
runtime recorded in the installation receipt. If Redux was selected, dictation
is disabled and the config moves to Base English first, so the daemon never
continues with deleted files. Removing unused Redux preserves another selected
model. Modified files are reported and preserved; unrelated models and shared
external native libraries are never removed. A previous user-installed adapter
is adopted after recognizing its files; its external native library remains
outside the ownership receipt.

Both the previous local service override and the portable adapter installer's
profile override migrate to `.pre-hub` backups. Every selected model then uses
the Hub-managed `voxtype/config.toml`.

## Cancellation

Every model installation shows Cancel. Cancellation terminates the installer's
whole child process group (including native build tools), waits for it, removes
staging, leaves the previous selection unchanged, and reports cancellation.
Whisper downloads also run in isolated staging and publish atomically.
Cancellation before subprocess startup is remembered and sent on startup.
Crashes are never treated as successful installations.

All slow Redux download/build/copy work happens before publication. Publication
uses exclusive complete-file links and a receipt; a late cancellation can leave
a complete installation cached without selecting it. It never replaces an
existing user model. Modified installed files cannot be silently uninstalled.

## Local delivery and upstream delivery

Source is on branch `local/redux-dictation`. The backend embeds the adapter,
installer and license/attribution files, so the existing Hub package build ships
them without relying on this machine's Projects directory. The supported
installation path requires a systemd user session and the build tools above;
other init systems keep the standard catalogue.

For the local extension, `~/.local/bin/ryoku-hub` forwards only `voxtype` commands
to `~/.local/libexec/ryoku-hub-redux`; other commands use the packaged backend.
The changed Dictation QML is deployed from this checkout into the explicit
`user_edits/quickshell/hub/pages/DictationPage.qml` overlay, applied using
`ryoku materialize`. Remove the launcher and this one overlay to return to the
packaged implementation. Upstream packages instead ship the changed backend
and QML directly; the local launcher and overlay are not required there.

Adapter upstream: https://github.com/Sipper1236/voxtype-redux-adapter .
Pinned source revisions, weight hash and licenses are recorded in
`ryoku/hub/backend/assets/redux/UPSTREAM.md`.

## Validation on 2026-10-18

- Full Hub Go tests and vet passed; the package-style CGO-disabled vendor build
  succeeded. Delivery and compositor-isolation checks passed.
- Five installer tests cover install/remove/reinstall, interrupted download,
  rollback on publication failure, edited-file preservation, and invalid receipts.
- A process-group test verifies even SIGTERM-ignoring descendants are killed.
- Two offscreen Quickshell tests exercise the actual page's cancellation handler,
  both after startup and before startup, preserving selection and cleaning staging.
- A fresh isolated home built the pinned native library from source, downloaded
  the hash-verified model, installed the adapter and transcribed the clean 5s clip
  correctly. This used the new 213 MB published GGUF; the existing local 318 MB
  conversion remains unchanged. It does not establish identical benchmark scores.
- An isolated lifecycle test exercised select, switch away with files retained,
  reselect, disable and minus removal. For that test only the host service boundary
  was mocked, preventing interference with live desktop services. The fresh
  installed worker/transcription itself ran for real.
- A real stock Tiny English download published atomically through the new staged
  path; a canceled real Redux install left no files/staging and kept its selection.

Reproduce the automated checks from the backend directory:
`go test ./...`, `go vet ./...`, and
`python -m unittest discover -s assets/redux -p '*_test.py'`.
Build the Hub, then run `RYOKU_TEST_HUB_BIN=/absolute/path/to/ryoku-hub python
 tests/dictation-cancel.py` from the repo root for the offscreen tests.
